// Gosint, Copyright (c) 2026 YouSysAdmin

package postgres_test

import (
	"encoding/json/jsontext"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/database/postgres"
)

// Two nodes, one database: what one publishes reaches a stream on the
// other, once, and not back to itself twice.
func TestTheRelayCarriesEventsBetweenNodes(t *testing.T) {
	dsn := os.Getenv(dbtest.PostgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set", dbtest.PostgresDSNEnv)
	}

	p, err := postgres.Open(t.Context(), dsn, database.Pool{})
	if err != nil {
		t.Fatal(err)
	}

	defer p.Close() //nolint:errcheck // test

	v, err := vault.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.DiscardHandler)
	busA, busB := eventbus.New(), eventbus.New()
	postgres.NewRelay(p, dsn, "node-a", busA, v, log).Start(t.Context())
	postgres.NewRelay(p, dsn, "node-b", busB, v, log).Start(t.Context())

	onA, _ := busA.Subscribe("sk")
	onB, _ := busB.Subscribe("sk")

	// LISTEN is asynchronous, so publish until B hears it.
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			busA.Publish(eventbus.Event{Type: eventbus.TypeLog, SketchID: "sk", Data: map[string]string{"m": "hi"}})
		case e := <-onB.C:
			if string(e.Data.(jsontext.Value)) != `{"m":"hi"}` {
				t.Errorf("data on node b = %s", e.Data)
			}

			for len(onA.C) > 0 {
				got := <-onA.C
				if _, raw := got.Data.(jsontext.Value); raw {
					t.Fatal("node a received its own event back through the relay")
				}
			}

			return
		case <-deadline:
			t.Fatal("node b never received the event")
		}
	}
}
