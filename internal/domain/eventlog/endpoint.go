// Gosint, Copyright (c) 2026 YouSysAdmin

package eventlog

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/paging"
	"github.com/yousysadmin/gosint/internal/core/response"
	sessionmodel "github.com/yousysadmin/gosint/internal/models/session"
)

// heartbeat keeps an idle stream open through proxies that cut silent
// connections.
const heartbeat = 25 * time.Second

// MaxStreamLife recycles a stream. EventSource reconnects on its own,
// and a forgotten tab stops holding a goroutine. The server raises the
// write deadline of streaming routes above this.
const MaxStreamLife = 30 * time.Minute

// Handler serves a sketch's log and live stream. Every route is behind
// requireSketch.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the newest log lines, after ?since= when given.
func (h Handler) List(c fiber.Ctx) error {
	var since *time.Time
	if raw := c.Query("since"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return response.BadRequest(c, "since must be an RFC 3339 time")
		}

		since = &t
	}

	logs, err := h.Runtime.Store.Log.List(c, access.Sketch(c).ID, since, paging.Limit(c, 200, 2000))
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, logsResponse{Logs: logs})
}

// Clear empties the sketch's log.
func (h Handler) Clear(c fiber.Ctx) error {
	n, err := h.Runtime.Store.Log.DeleteBySketch(c, access.Sketch(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, clearedResponse{Count: n})
}

// Stream sends the sketch's events as server-sent events: "log" for a
// log line, "scan" for a scan changing status.
//
// The session cookie authenticates it, which EventSource sends on a
// same-origin request, so no token ever travels in the URL.
func (h Handler) Stream(c fiber.Ctx) error {
	sk := access.Sketch(c)
	sketchID := sk.ID
	still := h.stillAllowed(access.Session(c), access.User(c).ID, sk.InvestigationID)
	sub, err := h.Runtime.Events.Subscribe(sketchID)
	if err != nil {
		return response.TooManyRequests(c, err.Error())
	}

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	log := h.Runtime.Log

	return c.SendStreamWriter(func(w *bufio.Writer) {
		// Runs after the handler returned, outside the recover middleware.
		defer func() {
			if r := recover(); r != nil {
				log.Error("event stream panicked", "sketch", sketchID, "reason", fmt.Sprint(r))
			}
		}()

		defer sub.Close()

		life := time.NewTimer(MaxStreamLife)
		defer life.Stop()

		ping := time.NewTicker(heartbeat)
		defer ping.Stop()

		if !write(w, "connected", connected{SketchID: sketchID}) {
			return
		}

		for {
			select {
			case e, ok := <-sub.C:
				if !ok || !write(w, e.Type, e.Data) {
					return
				}
			case <-ping.C:
				if !still() {
					write(w, "reconnect", reconnect{Reason: "access ended"})

					return
				}

				if _, err := w.WriteString(": ping\n\n"); err != nil || w.Flush() != nil {
					return
				}
			case <-life.C:
				_, _ = w.WriteString("retry: 1000\n")
				write(w, "reconnect", reconnect{Reason: "connection recycled"})

				return
			}
		}
	})
}

// stillAllowed is checked on every heartbeat: a stream outlives its
// request, and must end once the session is revoked or the member
// removed, not at its recycle half an hour later. A failed read keeps
// the stream, the next heartbeat asks again.
func (h Handler) stillAllowed(sess *sessionmodel.Session, userID, investigationID string) func() bool {
	return func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), recheckTimeout)
		defer cancel()

		if sess != nil {
			live, err := h.Runtime.Store.Session.Lookup(ctx, sess.TokenHash, time.Now().UTC())
			if err == nil && live == nil {
				return false
			}
		}

		role, err := h.Runtime.Store.Investigation.Role(ctx, investigationID, userID)

		return err != nil || role != ""
	}
}

// recheckTimeout bounds the access check of one heartbeat.
const recheckTimeout = 5 * time.Second

// write sends one frame. False when the reader has gone.
func write(w *bufio.Writer, event string, data any) bool {
	body, err := response.Marshal(data)
	if err != nil {
		return false
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, body); err != nil {
		return false
	}

	return w.Flush() == nil
}
