// Gosint, Copyright (c) 2026 YouSysAdmin

package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/live"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/store"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

func sketchFixture(t *testing.T, st *store.Store) string {
	t.Helper()

	ctx, now := t.Context(), time.Now().UTC()
	u := &usermodel.User{ID: ids.New(), Email: ids.New() + "@x.io", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
	inv := &invmodel.Investigation{ID: ids.New(), Name: "c", Status: "active", OwnerID: u.ID, CreatedAt: now, UpdatedAt: now}
	sk := &sketchmodel.Sketch{ID: ids.New(), InvestigationID: inv.ID, Title: "s", Status: "active", CreatedAt: now, UpdatedAt: now}

	if err := st.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	if err := st.Investigation.Create(ctx, inv); err != nil {
		t.Fatal(err)
	}

	if err := st.Sketch.Create(ctx, sk); err != nil {
		t.Fatal(err)
	}

	return sk.ID
}

type harness struct {
	st     *store.Store
	bus    *eventbus.Bus
	w      *worker.Worker
	sketch string
}

func newHarness(t *testing.T, db database.Database) *harness {
	t.Helper()

	st := backend.BindStore(db, nil)
	bus := eventbus.New()
	w := worker.New(worker.Config{
		Scans: st.Scan, Logs: st.Log, Bus: bus, Log: slog.New(slog.DiscardHandler),
		NodeID: "test-" + ids.New(),
		Tune: func() worker.Tuning {
			return worker.Tuning{Enabled: true, Concurrency: 2, Poll: 20 * time.Millisecond, Lease: time.Second, MaxAttempts: 2}
		},
	})

	return &harness{st: st, bus: bus, w: w, sketch: sketchFixture(t, st)}
}

func (h *harness) enqueue(t *testing.T, kind string) string {
	t.Helper()

	sc := &scanmodel.Scan{ID: ids.New(), SketchID: h.sketch, Kind: kind, Name: kind, Status: scanmodel.StatusPending,
		CreatedAt: time.Now().UTC()}
	if err := h.st.Scan.Enqueue(t.Context(), sc); err != nil {
		t.Fatal(err)
	}

	h.w.Wake()

	return sc.ID
}

func (h *harness) waitFor(t *testing.T, id string, want scanmodel.Status) *scanmodel.Scan {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sc, err := h.st.Scan.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}

		if sc.Status == want {
			return sc
		}

		time.Sleep(20 * time.Millisecond)
	}

	sc, _ := h.st.Scan.Get(t.Context(), id)
	t.Fatalf("scan %s is %s, want %s", id, sc.Status, want)

	return nil
}

func TestScansRunToTheirOutcome(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		h := newHarness(t, db)
		h.w.Handle("ok", func(_ context.Context, run *worker.Run) (map[string]any, error) {
			run.Log(logmodel.LevelInfo, "working", map[string]any{"step": 1.0})

			return map[string]any{"found": 2.0}, nil
		})
		h.w.Handle("bad", func(context.Context, *worker.Run) (map[string]any, error) {
			return nil, errors.New("the API said no")
		})
		h.w.Handle("boom", func(context.Context, *worker.Run) (map[string]any, error) {
			panic("bug")
		})

		sub, _ := h.bus.Subscribe(h.sketch)
		defer sub.Close()

		ctx, stop := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { h.w.Run(ctx); close(done) }()

		defer func() { stop(); <-done }()

		ok := h.waitFor(t, h.enqueue(t, "ok"), scanmodel.StatusCompleted)
		if ok.Details["found"] != 2.0 {
			t.Errorf("details = %v", ok.Details)
		}

		bad := h.waitFor(t, h.enqueue(t, "bad"), scanmodel.StatusFailed)
		if bad.Error != "the API said no" {
			t.Errorf("error = %q", bad.Error)
		}

		h.waitFor(t, h.enqueue(t, "boom"), scanmodel.StatusFailed)
		h.waitFor(t, h.enqueue(t, "unknown-kind"), scanmodel.StatusFailed)

		logs, err := h.st.Log.List(t.Context(), h.sketch, nil, 100)
		if err != nil {
			t.Fatal(err)
		}

		seen := map[logmodel.Level]bool{}
		for _, e := range logs {
			seen[e.Level] = true
		}

		for _, want := range []logmodel.Level{logmodel.LevelRunning, logmodel.LevelInfo, logmodel.LevelCompleted, logmodel.LevelFailed} {
			if !seen[want] {
				t.Errorf("no %s line in the log: %+v", want, logs)
			}
		}

		types := map[string]bool{}
		for len(sub.C) > 0 {
			types[(<-sub.C).Type] = true
		}

		if !types[eventbus.TypeLog] || !types[eventbus.TypeScan] {
			t.Errorf("stream saw %v, want log and scan events", types)
		}
	})
}

// Cancelling a running scan reaches its handler through the context.
func TestCancellingARunningScanStopsItsHandler(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		h := newHarness(t, db)
		started, stopped := make(chan struct{}), make(chan error, 1)
		h.w.Handle("slow", func(ctx context.Context, _ *worker.Run) (map[string]any, error) {
			close(started)
			<-ctx.Done()
			stopped <- context.Cause(ctx)

			return nil, ctx.Err()
		})

		ctx, stop := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { h.w.Run(ctx); close(done) }()

		defer func() { stop(); <-done }()

		id := h.enqueue(t, "slow")
		<-started

		if ok, err := h.st.Scan.Cancel(t.Context(), id, time.Now().UTC()); err != nil || !ok {
			t.Fatalf("cancel = %v, %v", ok, err)
		}

		select {
		case cause := <-stopped:
			if !errors.Is(cause, worker.ErrCancelled) {
				t.Errorf("cause = %v, want ErrCancelled", cause)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the handler never saw the cancellation")
		}

		h.waitFor(t, id, scanmodel.StatusCancelled)
	})
}

// Stopping the worker mid-run hands the scan back instead of failing it.
func TestShutdownHandsRunningScansBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		h := newHarness(t, db)
		started := make(chan struct{})
		h.w.Handle("slow", func(ctx context.Context, _ *worker.Run) (map[string]any, error) {
			close(started)
			<-ctx.Done()

			return nil, ctx.Err()
		})

		ctx, stop := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { h.w.Run(ctx); close(done) }()

		id := h.enqueue(t, "slow")
		<-started
		stop()
		<-done

		sc, err := h.st.Scan.Get(t.Context(), id)
		if err != nil || sc.Status != scanmodel.StatusPending {
			t.Errorf("after shutdown: %+v, %v, want pending", sc, err)
		}
	})
}

// TestMain fails the package when a test leaves a goroutine behind: a
// lease renewer or a stream that outlives its run would keep touching
// rows another node owns.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// flakyScans is the scan store with renewals failing on demand, the
// database being unreachable from the worker's point of view.
type flakyScans struct {
	store.ScanStore
	failing atomic.Bool
}

func (f *flakyScans) Renew(ctx context.Context, id, worker string, lease time.Time) (bool, error) {
	if f.failing.Load() {
		return false, errors.New("connection refused")
	}

	return f.ScanStore.Renew(ctx, id, worker, lease)
}

// A run whose renewals fail for a whole lease is stopped with
// ErrLeaseLost: by then the queue may have handed the scan to another
// worker, and two runs of one scan would write the graph twice.
func TestARunWhoseLeaseCannotBeRenewedStops(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st := backend.BindStore(db, nil)
		scans := &flakyScans{ScanStore: st.Scan}
		bus := eventbus.New()
		w := worker.New(worker.Config{
			Scans: scans, Logs: st.Log, Bus: bus, Log: slog.New(slog.DiscardHandler),
			NodeID: "test-" + ids.New(),
			Tune: func() worker.Tuning {
				return worker.Tuning{Enabled: true, Concurrency: 1, Poll: 20 * time.Millisecond, Lease: 150 * time.Millisecond, MaxAttempts: 5}
			},
		})
		sketch := sketchFixture(t, st)

		causes := make(chan error, 8)
		w.Handle("slow", func(ctx context.Context, _ *worker.Run) (map[string]any, error) {
			<-ctx.Done()
			causes <- context.Cause(ctx)

			return nil, ctx.Err()
		})

		ctx, stop := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { w.Run(ctx); close(done) }()

		defer func() { stop(); <-done }()

		scans.failing.Store(true)
		sc := &scanmodel.Scan{ID: ids.New(), SketchID: sketch, Kind: "slow", Name: "slow", Status: scanmodel.StatusPending,
			CreatedAt: time.Now().UTC()}
		if err := st.Scan.Enqueue(t.Context(), sc); err != nil {
			t.Fatal(err)
		}

		w.Wake()

		select {
		case cause := <-causes:
			if !errors.Is(cause, worker.ErrLeaseLost) || !errors.Is(cause, worker.ErrCancelled) {
				t.Errorf("cause = %v, want ErrLeaseLost wrapping ErrCancelled", cause)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the run went on without a lease")
		}

		stop()
		<-done

		// The run did not settle the scan: the queue still holds it and
		// hands it out again.
		got, err := st.Scan.Get(t.Context(), sc.ID)
		if err != nil {
			t.Fatal(err)
		}

		if got.Status != scanmodel.StatusRunning && got.Status != scanmodel.StatusPending {
			t.Errorf("status = %s, want running or pending", got.Status)
		}
	})
}

// A disabled worker leaves the queue alone, and switching it on lets it
// claim without a restart.
func TestADisabledWorkerWaits(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		h := newHarness(t, db)
		tuning := live.New(worker.Tuning{Concurrency: 1, Poll: 20 * time.Millisecond, Lease: time.Second, MaxAttempts: 1})
		h.w = worker.New(worker.Config{
			Scans: h.st.Scan, Logs: h.st.Log, Bus: h.bus, Log: slog.New(slog.DiscardHandler),
			NodeID: "test-" + ids.New(), Tune: tuning.Get,
		})
		h.w.Handle("ok", func(context.Context, *worker.Run) (map[string]any, error) { return nil, nil })

		ctx, stop := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { h.w.Run(ctx); close(done) }()

		defer func() { stop(); <-done }()

		id := h.enqueue(t, "ok")
		time.Sleep(150 * time.Millisecond)
		h.waitFor(t, id, scanmodel.StatusPending)

		on := tuning.Get()
		on.Enabled = true
		tuning.Set(on)
		h.waitFor(t, id, scanmodel.StatusCompleted)
	})
}
