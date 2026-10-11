// Gosint, Copyright (c) 2026 YouSysAdmin

// Package worker runs queued scans.
//
// The queue is the scans table. A worker claims pending scans up to its
// concurrency, holds each by renewing its lease, and settles it when
// the handler returns. Cancelling a scan is a row change the next
// renewal notices. A worker that dies leaves leases that run out, and
// any worker puts those scans back.
//
// Several workers, in one process or on several nodes, share one queue
// safely: Claim only ever hands a scan to one of them.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/domain/store"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// ErrCancelled is the cause of a run's context when its scan was
// cancelled or taken back.
var ErrCancelled = errors.New("worker: scan cancelled")

// ErrLeaseLost is the cause when the lease could not be renewed for a
// whole lease: the queue has handed the scan to somebody else by now.
// It wraps ErrCancelled, so a handler checks one cause.
var ErrLeaseLost = fmt.Errorf("%w: lease lost", ErrCancelled)

// logTimeout bounds writing one log line.
const logTimeout = 5 * time.Second

// Handler runs one scan and returns its details.
type Handler func(ctx context.Context, run *Run) (map[string]any, error)

// Tuning is what an administrator may change while the worker runs.
type Tuning struct {
	// Enabled lets the workers claim scans. Off, scans wait queued.
	Enabled bool

	// Concurrency is how many scans run at once on a node.
	Concurrency int

	// Poll is how often the queue is checked when nothing woke the
	// worker, which is how scans queued by another node are found.
	Poll time.Duration

	// Lease is how long a claimed scan stays claimed without the worker
	// renewing it. A scan whose worker died is retried after it.
	Lease time.Duration

	// MaxAttempts is how many times a scan whose worker died is retried.
	MaxAttempts int
}

// DefaultTuning is the tuning of an install that saved none.
func DefaultTuning() Tuning {
	return Tuning{Enabled: true, Concurrency: 4, Poll: 2 * time.Second, Lease: 2 * time.Minute, MaxAttempts: 3}
}

// Config is what a worker needs.
type Config struct {
	Scans  store.ScanStore
	Logs   store.LogStore
	Bus    *eventbus.Bus
	Log    *slog.Logger
	NodeID string

	// Tune reads the tuning, on every pass, so a change applies without
	// a restart. Nil runs on DefaultTuning.
	Tune func() Tuning

	// Paused, when set and true, keeps the worker from claiming: a
	// sealed server cannot open what a scan reads.
	Paused func() bool
}

// Worker claims and runs scans.
type Worker struct {
	cfg      Config
	handlers map[string]Handler
	wake     chan struct{}

	// active counts the scans running now.
	active atomic.Int64
}

// New builds a worker. Register handlers before Run.
func New(cfg Config) *Worker {
	return &Worker{cfg: cfg, handlers: map[string]Handler{}, wake: make(chan struct{}, 1)}
}

// tuning is the tuning now, a value no run can live with replaced.
func (w *Worker) tuning() Tuning {
	t := DefaultTuning()
	if w.cfg.Tune != nil {
		t = w.cfg.Tune()
	}

	d := DefaultTuning()
	t.Concurrency = max(t.Concurrency, 1)
	t.MaxAttempts = max(t.MaxAttempts, 1)
	if t.Poll <= 0 {
		t.Poll = d.Poll
	}

	if t.Lease <= 0 {
		t.Lease = d.Lease
	}

	return t
}

// Handle registers the handler for a scan kind.
func (w *Worker) Handle(kind string, h Handler) { w.handlers[kind] = h }

// Wake makes the worker look at the queue now instead of at the next poll.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run claims and runs scans until ctx ends, then hands the ones still
// running back to the queue and returns.
func (w *Worker) Run(ctx context.Context) {
	var (
		wg      sync.WaitGroup
		poll    = w.tuning().Poll
		tick    = time.NewTicker(poll)
		running = context.WithoutCancel(ctx)
	)

	defer tick.Stop()

	stopRuns, cancelRuns := context.WithCancelCause(running)
	defer cancelRuns(nil)

	for {
		t := w.tuning()
		if t.Poll != poll {
			poll = t.Poll
			tick.Reset(poll)
		}

		if t.Enabled {
			w.recoverExpired(ctx, t)
			w.claim(ctx, stopRuns, t, &wg)
		}

		select {
		case <-ctx.Done():
			cancelRuns(context.Cause(ctx))
			wg.Wait()

			return
		case <-tick.C:
		case <-w.wake:
		}
	}
}

func (w *Worker) recoverExpired(ctx context.Context, t Tuning) {
	requeued, failed, err := w.cfg.Scans.RecoverExpired(ctx, time.Now().UTC(), t.MaxAttempts)
	if err != nil {
		if ctx.Err() == nil {
			w.cfg.Log.Warn("worker: recovering expired scans failed", "err", err)
		}

		return
	}

	if requeued+failed > 0 {
		w.cfg.Log.Info("worker: recovered scans whose worker stopped", "requeued", requeued, "failed", failed)
	}
}

func (w *Worker) claim(ctx, runs context.Context, t Tuning, wg *sync.WaitGroup) {
	free := t.Concurrency - int(w.active.Load())
	if free <= 0 || ctx.Err() != nil || (w.cfg.Paused != nil && w.cfg.Paused()) {
		return
	}

	now := time.Now().UTC()
	claimed, err := w.cfg.Scans.Claim(ctx, w.cfg.NodeID, now, now.Add(t.Lease), free)
	if err != nil && ctx.Err() == nil {
		w.cfg.Log.Warn("worker: claiming scans failed", "err", err)
	}

	for _, sc := range claimed {
		w.active.Add(1)
		wg.Go(func() {
			defer w.active.Add(-1)

			w.execute(runs, &sc)
		})
	}
}

// execute runs one claimed scan to its end.
func (w *Worker) execute(parent context.Context, sc *scanmodel.Scan) {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)

	run := &Run{Scan: sc, w: w}
	w.publishScan(sc)
	run.Log(logmodel.LevelRunning, fmt.Sprintf("%s %s started", sc.Kind, sc.Name), nil)

	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)

		w.renew(ctx, sc.ID, cancel)
	}()

	details, err := w.call(ctx, run)
	cancel(nil)
	<-renewDone

	if cause := context.Cause(ctx); errors.Is(cause, ErrCancelled) {
		verb := "was cancelled"
		if errors.Is(cause, ErrLeaseLost) {
			verb = "lost its lease and will run again"
		}

		run.Log(logmodel.LevelWarning, fmt.Sprintf("%s %s %s", sc.Kind, sc.Name, verb), nil)

		return
	}

	if err != nil && parent.Err() != nil {
		// Interrupted by the shutdown: hand the scan back so a worker
		// retries it. One that finished anyway is settled below.
		if _, err := w.cfg.Scans.Release(context.WithoutCancel(parent), sc.ID, w.cfg.NodeID); err != nil {
			w.cfg.Log.Warn("worker: releasing a scan on shutdown failed, its lease will expire", "scan", sc.ID, "err", err)
		}

		return
	}

	now := time.Now().UTC()
	sc.CompletedAt = &now
	sc.Details = details
	sc.Status = scanmodel.StatusCompleted
	if err != nil {
		sc.Status = scanmodel.StatusFailed
		sc.Error = err.Error()
	}

	settled, ferr := w.cfg.Scans.Finish(context.WithoutCancel(parent), sc, w.cfg.NodeID)
	if ferr != nil {
		w.cfg.Log.Error("worker: settling a scan failed", "scan", sc.ID, "err", ferr)

		return
	}

	if !settled {
		return
	}

	if err != nil {
		run.Log(logmodel.LevelFailed, fmt.Sprintf("%s %s failed: %s", sc.Kind, sc.Name, err), nil)
	} else {
		run.Log(logmodel.LevelCompleted, fmt.Sprintf("%s %s completed", sc.Kind, sc.Name), nil)
	}

	w.publishScan(sc)
}

// call runs the handler, turning a panic into a failure.
func (w *Worker) call(ctx context.Context, run *Run) (details map[string]any, err error) {
	h, ok := w.handlers[run.Scan.Kind]
	if !ok {
		return nil, fmt.Errorf("no handler for scan kind %q", run.Scan.Kind)
	}

	defer func() {
		if r := recover(); r != nil {
			w.cfg.Log.Error("worker: handler panicked", "scan", run.Scan.ID, "reason", fmt.Sprint(r),
				"stack", string(debug.Stack()))
			err = fmt.Errorf("internal error in %s %s", run.Scan.Kind, run.Scan.Name)
		}
	}()

	return h(ctx, run)
}

// renew holds the lease until ctx ends, cancelling the run when the
// scan is no longer this worker's. A renewal that keeps failing counts
// the same once a whole lease has passed: the lease has run out by
// then and another claim may hold the scan, so going on would run it
// twice.
func (w *Worker) renew(ctx context.Context, id string, cancel context.CancelCauseFunc) {
	lease := w.tuning().Lease
	t := time.NewTicker(lease / 3)
	defer t.Stop()

	held := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		ok, err := w.cfg.Scans.Renew(ctx, id, w.cfg.NodeID, time.Now().UTC().Add(lease))
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			w.cfg.Log.Warn("worker: renewing a lease failed", "scan", id, "err", err)
			if time.Since(held) >= lease {
				cancel(ErrLeaseLost)

				return
			}

			continue
		}

		if !ok {
			cancel(w.lossCause(ctx, id))

			return
		}

		held = time.Now()
	}
}

// lossCause tells a scan somebody cancelled from one the queue took
// back, by the row's status. Unknown reads as cancelled.
func (w *Worker) lossCause(ctx context.Context, id string) error {
	sc, err := w.cfg.Scans.Get(ctx, id)
	if err != nil || sc == nil || sc.Status == scanmodel.StatusCancelled {
		return ErrCancelled
	}

	return ErrLeaseLost
}

func (w *Worker) publishScan(sc *scanmodel.Scan) {
	if sc.SketchID == "" {
		return
	}

	w.cfg.Bus.Publish(eventbus.Event{Type: eventbus.TypeScan, SketchID: sc.SketchID, Data: *sc})
}

// Run is one scan in progress, handed to its handler.
type Run struct {
	Scan *scanmodel.Scan
	w    *Worker
}

// Log writes a line to the sketch's log and its live stream. A run on
// no sketch logs to the process log only. Never fails the run.
func (r *Run) Log(level logmodel.Level, message string, data map[string]any) {
	if r.Scan.SketchID == "" {
		r.w.cfg.Log.Info("scan log", "scan", r.Scan.ID, "level", level, "message", message)

		return
	}

	e := &logmodel.Entry{
		ID:        ids.New(),
		SketchID:  r.Scan.SketchID,
		ScanID:    r.Scan.ID,
		Level:     level,
		Message:   message,
		Data:      data,
		CreatedAt: time.Now().UTC(),
	}
	// Not the run's context: a cancelled run still logs that it was.
	ctx, cancel := context.WithTimeout(context.Background(), logTimeout)
	defer cancel()

	if err := r.w.cfg.Logs.Append(ctx, e); err != nil {
		r.w.cfg.Log.Warn("worker: writing a log line failed", "scan", r.Scan.ID, "err", err)
	}

	r.w.cfg.Bus.Publish(eventbus.Event{Type: eventbus.TypeLog, SketchID: e.SketchID, Data: *e})
}
