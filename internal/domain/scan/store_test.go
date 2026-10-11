// Gosint, Copyright (c) 2026 YouSysAdmin

package scan_test

import (
	"sync"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// noVault stores clear text, the queue does not depend on sealing.
var noVault *vault.Vault

func enqueue(t *testing.T, st *scan.Store, n int) []string {
	t.Helper()

	out := make([]string, 0, n)
	for i := range n {
		sc := &scanmodel.Scan{ID: ids.New(), Kind: "test", Name: "noop", Status: scanmodel.StatusPending,
			Input: map[string]any{"i": i}, CreatedAt: time.Now().UTC()}
		if err := st.Enqueue(t.Context(), sc); err != nil {
			t.Fatal(err)
		}

		out = append(out, sc.ID)
	}

	return out
}

// Several workers claiming at once must never get the same scan.
func TestEveryScanIsClaimedOnce(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st := scan.NewStore(db, noVault)
		enqueue(t, st, 40)

		var (
			mu  sync.Mutex
			got = map[string]string{}
			wg  sync.WaitGroup
		)

		for w := range 4 {
			wg.Go(func() {
				worker := "w" + string(rune('a'+w)) //nolint:gosec // four workers
				for {
					now := time.Now().UTC()
					claimed, err := st.Claim(t.Context(), worker, now, now.Add(time.Minute), 3)
					if err != nil {
						t.Error(err)

						return
					}

					if len(claimed) == 0 {
						return
					}

					mu.Lock()
					for _, c := range claimed {
						if prev, dup := got[c.ID]; dup {
							t.Errorf("scan %s claimed by %s and %s", c.ID, prev, worker)
						}

						got[c.ID] = worker
					}
					mu.Unlock()
				}
			})
		}

		wg.Wait()
		if len(got) != 40 {
			t.Errorf("%d scans claimed, want 40", len(got))
		}
	})
}

func TestACancelledScanStopsRenewing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st := scan.NewStore(db, noVault)
		ctx := t.Context()
		id := enqueue(t, st, 1)[0]
		now := time.Now().UTC()

		if c, err := st.Claim(ctx, "w", now, now.Add(time.Minute), 1); err != nil || len(c) != 1 {
			t.Fatalf("claim = %v, %v", c, err)
		}

		if ok, err := st.Renew(ctx, id, "w", now.Add(2*time.Minute)); err != nil || !ok {
			t.Fatalf("renew = %v, %v", ok, err)
		}

		if ok, err := st.Renew(ctx, id, "someone-else", now.Add(2*time.Minute)); err != nil || ok {
			t.Errorf("another worker renewed: %v, %v", ok, err)
		}

		if ok, err := st.Cancel(ctx, id, now); err != nil || !ok {
			t.Fatalf("cancel = %v, %v", ok, err)
		}

		if ok, _ := st.Renew(ctx, id, "w", now.Add(3*time.Minute)); ok {
			t.Error("a cancelled scan was renewed")
		}

		done := &scanmodel.Scan{ID: id, Status: scanmodel.StatusCompleted, CompletedAt: new(now)}
		if ok, _ := st.Finish(ctx, done, "w"); ok {
			t.Error("a cancelled scan was settled as completed")
		}

		sc, _ := st.Get(ctx, id)
		if sc.Status != scanmodel.StatusCancelled {
			t.Errorf("status = %s", sc.Status)
		}
	})
}

// A worker that died mid-run gives its scan back, until the attempts
// run out.
func TestAnExpiredLeaseIsRecovered(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st := scan.NewStore(db, noVault)
		ctx := t.Context()
		id := enqueue(t, st, 1)[0]
		start := time.Now().UTC()

		for attempt := 1; attempt <= 2; attempt++ {
			now := start.Add(time.Duration(attempt) * time.Hour)
			if c, err := st.Claim(ctx, "dead", now, now.Add(time.Minute), 1); err != nil || len(c) != 1 {
				t.Fatalf("attempt %d claim = %v, %v", attempt, c, err)
			}

			requeued, failed, err := st.RecoverExpired(ctx, now.Add(2*time.Minute), 2)
			if err != nil {
				t.Fatal(err)
			}

			want := [2]int64{1, 0}
			if attempt == 2 {
				want = [2]int64{0, 1}
			}

			if [2]int64{requeued, failed} != want {
				t.Errorf("attempt %d: requeued %d failed %d, want %v", attempt, requeued, failed, want)
			}
		}

		sc, _ := st.Get(ctx, id)
		if sc.Status != scanmodel.StatusFailed || sc.Attempts != 2 || sc.Error == "" {
			t.Errorf("after recovery: %+v", sc)
		}
	})
}

func TestFinishStoresTheOutcome(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st := scan.NewStore(db, noVault)
		ctx := t.Context()
		id := enqueue(t, st, 1)[0]
		now := time.Now().UTC()

		if _, err := st.Claim(ctx, "w", now, now.Add(time.Minute), 1); err != nil {
			t.Fatal(err)
		}

		done := &scanmodel.Scan{ID: id, Status: scanmodel.StatusCompleted, CompletedAt: new(now),
			Details: map[string]any{"nodes": 3.0}}
		if ok, err := st.Finish(ctx, done, "w"); err != nil || !ok {
			t.Fatalf("finish = %v, %v", ok, err)
		}

		sc, _ := st.Get(ctx, id)
		if sc.Status != scanmodel.StatusCompleted || sc.Details["nodes"] != 3.0 || sc.CompletedAt == nil || sc.Input["i"] != 0.0 {
			t.Errorf("settled scan = %+v", sc)
		}
	})
}
