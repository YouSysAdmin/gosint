// Gosint, Copyright (c) 2026 YouSysAdmin

// Package cron runs the scheduled maintenance jobs.
//
// The schedule vocabulary is deliberately two shapes - "daily at
// HH:MM UTC" and "every N" - rather than full cron expressions. That
// covers every job there is and keeps the dependency list unchanged. Schedules carry a cron-syntax Display string so the
// admin API can show operators something familiar.
//
// Jobs are expected to be idempotent and safe to skip: a node that is
// down at 03:00 does not backfill, it simply runs at 03:00 the next
// day. On a multi-node deployment every node runs every job, so jobs
// must tolerate concurrent execution - the built-in ones are all
// delete-by-age statements, which are naturally convergent.
package cron

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// Schedule decides when a job runs next.
type Schedule struct {
	// Daily, when set, runs the job once a day at this UTC time.
	Daily *TimeOfDay

	// Every, when set, runs the job on a fixed interval.
	Every time.Duration

	// Display is the human-facing form, cron syntax where it maps.
	Display string
}

// TimeOfDay is a UTC wall-clock time.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// DailyAt builds a schedule that fires once a day at hour:minute UTC.
func DailyAt(hour, minute int) Schedule {
	return Schedule{
		Daily:   &TimeOfDay{Hour: hour, Minute: minute},
		Display: fmt.Sprintf("%d %d * * *", minute, hour),
	}
}

// EveryInterval builds a fixed-interval schedule.
func EveryInterval(d time.Duration) Schedule {
	return Schedule{Every: d, Display: "every " + d.String()}
}

// Next returns the first firing strictly after from.
func (s Schedule) Next(from time.Time) time.Time {
	if s.Every > 0 {
		return from.Add(s.Every)
	}

	if s.Daily == nil {
		// A schedule with neither shape never fires. Park it far out
		// rather than spinning.
		return from.Add(24 * time.Hour)
	}

	from = from.UTC()
	next := time.Date(from.Year(), from.Month(), from.Day(),
		s.Daily.Hour, s.Daily.Minute, 0, 0, time.UTC)
	if !next.After(from) {
		next = next.AddDate(0, 0, 1)
	}

	return next
}

// Job is one scheduled unit of work.
type Job struct {
	Name     string
	Schedule Schedule

	// Run does the work. It should be idempotent.
	Run func(ctx context.Context) error
}

// Status is the admin-facing view of a job.
type Status struct {
	Name      string     `json:"name"`
	Schedule  string     `json:"schedule"`
	Running   bool       `json:"running"`
	LastRunAt *time.Time `json:"last_run_at"`
	LastError string     `json:"last_error"`
	NextRunAt *time.Time `json:"next_run_at"`

	// LastDuration is how long the previous run took, in
	// milliseconds.
	LastDurationMS int64 `json:"last_duration_ms"`
}

type jobState struct {
	job       Job
	running   bool
	lastRun   *time.Time
	lastErr   string
	nextRun   time.Time
	lastTaken time.Duration
}

// Manager owns the job set and the ticker driving it.
type Manager struct {
	log *slog.Logger

	mu   sync.RWMutex
	jobs map[string]*jobState

	// order preserves registration order for a stable listing.
	order []string

	stopped chan struct{}
	once    sync.Once

	// running counts jobs in flight, so shutdown can wait for them.
	running sync.WaitGroup

	// cut ends the jobs in flight once Wait has run out of patience.
	cut context.CancelFunc

	// tick is the scheduler's resolution, coarse because nothing here is
	// finer than hourly. Tests shorten it.
	tick time.Duration
}

// New builds an empty manager.
func New(log *slog.Logger) *Manager {
	return &Manager{
		log:     log,
		jobs:    map[string]*jobState{},
		stopped: make(chan struct{}),
		tick:    30 * time.Second,
	}
}

// Register adds a job. Call before Start.
func (m *Manager) Register(j Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.jobs[j.Name]; exists {
		return
	}

	m.jobs[j.Name] = &jobState{job: j, nextRun: j.Schedule.Next(time.Now().UTC())}
	m.order = append(m.order, j.Name)
}

// Start runs the scheduler until ctx is cancelled. The tick is coarse
// (30s) because nothing registered here is finer than hourly - firing a
// few seconds late is irrelevant, and a slow tick keeps an idle process
// genuinely idle.
//
// A job runs on a context that keeps ctx's values and drops its
// cancellation: cancelling ctx stops the SCHEDULING, and the job in
// flight finishes under Wait's timeout, which is what cuts it. Cut at
// the same instant as the loop, a retention sweep that had deleted a
// blob failed on the row DELETE naming it - the exact state Wait exists
// to prevent.
func (m *Manager) Start(ctx context.Context) {
	defer m.once.Do(func() { close(m.stopped) })

	flight, cut := context.WithCancel(context.WithoutCancel(ctx))
	m.mu.Lock()
	m.cut = cut
	jobs := len(m.jobs)
	m.mu.Unlock()

	t := time.NewTicker(m.tick)
	defer t.Stop()

	m.log.Info("cron: started", "jobs", jobs)

	for {
		select {
		case <-ctx.Done():
			m.log.Info("cron: stopped", "cause", context.Cause(ctx))

			return
		case <-t.C:
			m.runDue(flight)
		}
	}
}

// runDue fires every job whose time has come. Each runs in its own
// goroutine so a slow job does not hold up the others.
func (m *Manager) runDue(ctx context.Context) {
	now := time.Now().UTC()

	m.mu.Lock()
	var due []*jobState
	for _, name := range m.order {
		st := m.jobs[name]
		if st.running || now.Before(st.nextRun) {
			continue
		}

		st.running = true
		// Advance the clock immediately so a job that outlives its
		// own interval is not queued up again the moment it returns.
		st.nextRun = st.job.Schedule.Next(now)
		due = append(due, st)
	}

	m.mu.Unlock()

	for _, st := range due {
		// The return value is for RunNow. Here the outcome is already
		// logged and recorded on the job state by execute itself.
		m.running.Go(func() { _ = m.execute(ctx, st) })
	}
}

// Refusals RunNow reports before running anything, so a caller can
// tell them from the job's own failure.
var (
	ErrUnknownJob = errors.New("unknown job")
	ErrJobRunning = errors.New("job is already running")
)

// RunNow executes one job out of band, for the admin trigger. It
// reports ErrUnknownJob or ErrJobRunning without running anything, and
// otherwise what the job itself returned.
func (m *Manager) RunNow(ctx context.Context, name string) error {
	m.mu.Lock()
	st, ok := m.jobs[name]
	if !ok {
		m.mu.Unlock()

		return fmt.Errorf("%w %q", ErrUnknownJob, name)
	}

	if st.running {
		m.mu.Unlock()

		return fmt.Errorf("%w: %q", ErrJobRunning, name)
	}

	st.running = true
	m.mu.Unlock()

	// execute's own return, not a re-read of st.lastErr: the moment
	// execute unlocks, the scheduler may start the same job again and
	// overwrite lastErr - so the re-read could report the OTHER run's
	// outcome to the admin who pressed the button.
	return m.execute(ctx, st)
}

// execute runs one job and returns what the job returned, with a panic
// folded in as an error.
func (m *Manager) execute(ctx context.Context, st *jobState) error {
	name := st.job.Name
	started := time.Now().UTC()
	m.log.Info("cron: job start", "job", name)

	// A panicking job must not take the process with it, and its stack
	// is logged where it happened.
	err := contained(m.log, name, func() error { return st.job.Run(ctx) })

	finished := time.Now().UTC()
	m.mu.Lock()
	st.running = false
	st.lastRun = &started
	st.lastTaken = finished.Sub(started)
	if err != nil {
		st.lastErr = err.Error()
	} else {
		st.lastErr = ""
	}

	m.mu.Unlock()

	if err != nil {
		m.log.Error("cron: job failed", "job", name, "err", err, "took", finished.Sub(started))

		return err
	}

	m.log.Info("cron: job done", "job", name, "took", finished.Sub(started))

	return nil
}

// Statuses returns the admin view, in registration order.
func (m *Manager) Statuses() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.order))
	for _, name := range m.order {
		st := m.jobs[name]
		next := st.nextRun
		out = append(out, Status{
			Name:           name,
			Schedule:       st.job.Schedule.Display,
			Running:        st.running,
			LastRunAt:      st.lastRun,
			LastError:      st.lastErr,
			NextRunAt:      &next,
			LastDurationMS: st.lastTaken.Milliseconds(),
		})
	}

	return out
}

// Wait blocks until the scheduler loop has exited and every job in
// flight has returned, bounded by timeout. A job cut mid-sweep can
// leave rows naming blobs that are gone.
func (m *Manager) Wait(timeout time.Duration) {
	deadline := time.After(timeout)
	select {
	case <-m.stopped:
	case <-deadline:
		return
	}

	done := make(chan struct{})
	go func() {
		m.running.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-deadline:
		m.log.Warn("cron: jobs still running at shutdown, cutting them")
	}

	m.mu.Lock()
	cut := m.cut
	m.mu.Unlock()
	if cut != nil {
		cut()
	}
}

// contained runs fn, turning a panic into an error with its stack in
// the log, so one bad job does not take the process down.
func contained(log *slog.Logger, name string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("cron: job panicked", "job", name, "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	return fn()
}
