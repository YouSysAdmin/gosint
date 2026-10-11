// Gosint, Copyright (c) 2026 YouSysAdmin

package location

import (
	"sync"
	"testing"
	"time"
)

// Requests from runs going on at once are spaced too, not only those of
// one run.
func TestThePaceHoldsAcrossRuns(t *testing.T) {
	var p pacer
	start := time.Now()

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if err := p.wait(t.Context(), 50*time.Millisecond); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()
	if took := time.Since(start); took < 100*time.Millisecond {
		t.Errorf("three requests went out within %v", took)
	}
}
