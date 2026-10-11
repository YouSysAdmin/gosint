// Gosint, Copyright (c) 2026 YouSysAdmin

package postgres

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/database"
)

// relayChannel is the NOTIFY channel events travel on.
const relayChannel = "gosint_events"

// maxPayload stays under postgres's 8000 byte NOTIFY limit. A bigger
// event travels without its data, and the reader refetches.
const maxPayload = 7800

const relayReconnectDelay = 5 * time.Second

// relaySendTimeout bounds one notify, so a wedged postgres cannot stall
// the worker or a handler publishing an event.
const relaySendTimeout = 2 * time.Second

// relayIdleTimeout is how long the listener waits for a notification
// before checking that its connection is still alive.
const relayIdleTimeout = time.Minute

// relayed is an event on the wire between nodes.
type relayed struct {
	Origin   string         `json:"origin"`
	Type     string         `json:"type"`
	SketchID string         `json:"sketch_id"`
	Data     jsontext.Value `json:"data"`
	At       time.Time      `json:"at"`
}

// relayAAD is what a relayed event is sealed for.
const relayAAD = "relay:" + relayChannel

// Relay carries bus events between nodes over LISTEN / NOTIFY, so a
// live stream on one node shows what a worker on another did. An event
// is sealed whole: postgres keeps the notify queue on disk.
type Relay struct {
	db     *sql.DB
	dsn    string
	nodeID string
	bus    *eventbus.Bus
	codec  database.Codec
	log    *slog.Logger
}

// NewRelay builds a relay for bus. nodeID tells this node's own events
// apart when they come back.
func NewRelay(p *Postgres, dsn, nodeID string, bus *eventbus.Bus, codec database.Codec, log *slog.Logger) *Relay {
	return &Relay{db: p.db, dsn: dsn, nodeID: nodeID, bus: bus, codec: codec, log: log}
}

// seal is msg as it travels.
func (r *Relay) seal(msg relayed) (string, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	return r.codec.SealText(relayAAD, string(payload))
}

// Start hooks the relay onto the bus and listens until ctx ends.
func (r *Relay) Start(ctx context.Context) {
	r.bus.SetRelay(func(e eventbus.Event) { r.send(ctx, e) })

	go r.listenLoop(ctx)
}

func (r *Relay) send(ctx context.Context, e eventbus.Event) {
	data, err := json.Marshal(e.Data)
	if err != nil {
		r.log.Warn("relay: event data does not marshal", "type", e.Type, "err", err)

		return
	}

	msg := relayed{Origin: r.nodeID, Type: e.Type, SketchID: e.SketchID, Data: data, At: e.At}
	payload, err := r.seal(msg)
	if err == nil && len(payload) > maxPayload {
		msg.Data = jsontext.Value(`{"truncated":true}`)
		payload, err = r.seal(msg)
	}

	if err != nil {
		r.log.Debug("relay: event not sealed, other nodes miss it", "type", e.Type, "err", err)

		return
	}

	// Publishers are workers and handlers, so the notify is bounded
	// and outlives the serve context: a cancellation during shutdown
	// must still reach the other nodes.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), relaySendTimeout)
	defer cancel()

	if _, err := r.db.ExecContext(ctx, `SELECT pg_notify($1, $2)`, relayChannel, payload); err != nil {
		r.log.Warn("relay: notify failed, other nodes miss this event", "type", e.Type, "err", err)
	}
}

func (r *Relay) listenLoop(ctx context.Context) {
	for ctx.Err() == nil {
		err := r.listenOnce(ctx)
		if ctx.Err() != nil {
			return
		}

		r.log.Warn("relay: listen dropped, reconnecting", "err", err, "retry_in", relayReconnectDelay.String())

		wait := time.NewTimer(relayReconnectDelay)
		select {
		case <-wait.C:
		case <-ctx.Done():
			wait.Stop()

			return
		}
	}
}

func (r *Relay) listenOnce(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, r.dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+relayChannel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	for {
		// A connection dropped without a FIN never errors on its own, so
		// the wait is bounded and the connection pinged when it runs out.
		wctx, cancel := context.WithTimeout(ctx, relayIdleTimeout)
		n, err := conn.WaitForNotification(wctx)
		cancel()
		if err != nil {
			if ctx.Err() != nil || wctx.Err() == nil {
				return err
			}

			pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Ping(pingCtx)
			cancelPing()
			if err != nil {
				return fmt.Errorf("ping: %w", err)
			}

			continue
		}

		r.receive(n.Payload)
	}
}

// receive delivers one notification. Anyone who can connect to the
// database can notify the channel, so nothing in a payload may stop the
// listener: a failure is logged and the next one read.
func (r *Relay) receive(payload string) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("relay: a notification failed", "panic", p)
		}
	}()

	plain, err := r.codec.OpenText(relayAAD, payload)
	if err != nil {
		r.log.Debug("relay: event not opened", "err", err)

		return
	}

	var msg relayed
	if err := json.Unmarshal([]byte(plain), &msg); err != nil {
		r.log.Warn("relay: unreadable event", "err", err)

		return
	}

	if msg.Origin == r.nodeID {
		return
	}

	r.bus.Deliver(eventbus.Event{Type: msg.Type, SketchID: msg.SketchID, Data: msg.Data, At: msg.At})
}
