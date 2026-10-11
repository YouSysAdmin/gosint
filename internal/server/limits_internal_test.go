// Gosint, Copyright (c) 2026 YouSysAdmin

package server

import (
	"context"
	"testing"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/live"
	"github.com/yousysadmin/gosint/internal/domain/auth"
	"github.com/yousysadmin/gosint/internal/domain/graph"
	"github.com/yousysadmin/gosint/internal/domain/store"
	sessionmodel "github.com/yousysadmin/gosint/internal/models/session"
)

// liveSessions knows one session, by any token "live".
type liveSessions struct{ store.SessionStore }

func (liveSessions) Lookup(_ context.Context, hash string, _ time.Time) (*sessionmodel.Session, error) {
	if hash == "" {
		return nil, nil
	}

	return &sessionmodel.Session{ID: "s"}, nil
}

// The three routes taking an export file may receive one as large as an
// export is written, every other write a file upload, and only from a
// caller holding a live session: without one nothing raises the small
// default, so a stranger cannot make the server hold a large body.
func TestBodiesAreLargeOnlyForASession(t *testing.T) {
	rt := &env.Runtime{Config: &env.Config{}, Store: &store.Store{Session: liveSessions{}}, FileLimits: live.New(env.DefaultFileLimits())}
	limits := perRequestLimits(rt)
	cases := []struct {
		method, uri string
		session     bool
		want        int
	}{
		{"POST", "/api/investigations/import", true, graph.ImportBodyLimit},
		{"POST", "/api/investigations/0190-a/sketches/import", true, graph.ImportBodyLimit},
		{"POST", "/api/sketches/0190-a/import/file?x=1", true, graph.ImportBodyLimit},
		{"POST", "/api/sketches/0190-a/import/table", true, bodyLimit(rt.FileLimits.Get().MaxSize)},
		{"POST", "/api/sketches//import/file", true, bodyLimit(rt.FileLimits.Get().MaxSize)},
		{"POST", "/api/investigations/import", false, 0},
		{"POST", "/api/sketches/0190-a/nodes/x/files", false, 0},
		{"GET", "/api/investigations/import", true, 0},
	}

	for _, tc := range cases {
		var h fasthttp.RequestHeader
		h.SetMethod(tc.method)
		h.SetRequestURI(tc.uri)
		if tc.session {
			h.SetCookie(auth.CookieName, "live")
		}

		if got := limits(&h).MaxRequestBodySize; got != tc.want {
			t.Errorf("%s %s session=%v: body limit %d, want %d", tc.method, tc.uri, tc.session, got, tc.want)
		}
	}

	if graph.ImportBodyLimit <= bodyLimit(0) || bodyLimit(0) <= anonymousBodyLimit {
		t.Error("the limits are not ordered anonymous < upload < import")
	}
}
