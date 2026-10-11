// Gosint, Copyright (c) 2026 YouSysAdmin

package scan

import (
	"context"
	"time"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// How Wait watches a scan: the table is read on every nudge and at
// least every pollEvery, the caller hears every progressEvery.
const (
	pollEvery     = 2 * time.Second
	progressEvery = 15 * time.Second
)

// Wait queues a scan and waits for it to settle, for a caller that wants
// the result in hand, such as the chat running a transform for the
// model. The subscription is opened BEFORE the scan is queued, so the
// settling event cannot slip past, and the table is polled too, since
// events are best effort and the relay carries a big one truncated. The
// table is the authority: an event only says when to look. progress,
// when given, is called every progressEvery with the time elapsed. When
// ctx ends the scan goes on and the last state read comes back with the
// context's error.
func Wait(ctx context.Context, rt *env.Runtime, sc *scanmodel.Scan, enqueue func() error,
	progress func(elapsed time.Duration)) (*scanmodel.Scan, error) {
	sub, err := rt.Events.Subscribe(sc.SketchID)
	if err != nil {
		return nil, err
	}

	defer sub.Close()

	if err := enqueue(); err != nil {
		return nil, err
	}

	start := time.Now()
	poll := time.NewTicker(pollEvery)
	defer poll.Stop()

	tick := time.NewTicker(progressEvery)
	defer tick.Stop()

	events, latest := sub.C, sc
	for {
		cur, err := rt.Store.Scan.Get(ctx, sc.ID)
		if err != nil {
			return latest, err
		}

		if cur != nil {
			latest = cur
			if cur.Status.Final() {
				return cur, nil
			}
		}

	wait:
		for {
			select {
			case <-ctx.Done():
				return latest, ctx.Err()
			case e, ok := <-events:
				if !ok {
					// The bus closed, the poll carries on alone.
					events = nil

					continue
				}

				if e.Type == eventbus.TypeScan && about(e, sc.ID) {
					break wait
				}
			case <-poll.C:
				break wait
			case <-tick.C:
				if progress != nil {
					progress(time.Since(start))
				}
			}
		}
	}
}

// about reports whether a scan event is about id, or cannot say: off
// the relay the payload is a map, or a truncation note, and either is
// reason enough to look.
func about(e eventbus.Event, id string) bool {
	switch d := e.Data.(type) {
	case scanmodel.Scan:
		return d.ID == id
	case *scanmodel.Scan:
		return d.ID == id
	case map[string]any:
		v, ok := d["id"].(string)

		return !ok || v == id
	default:
		return true
	}
}
