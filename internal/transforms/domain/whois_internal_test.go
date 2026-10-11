// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
)

// A WHOIS server that never answers holds a cancelled run no longer
// than the cancel takes.
func TestACancelEndsAWhoisRead(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer l.Close() //nolint:errcheck // test

	go func() {
		c, err := l.Accept()
		if err == nil {
			defer c.Close() //nolint:errcheck // test
			time.Sleep(5 * time.Second)
		}
	}()

	ctx, cancel := context.WithCancel(t.Context())
	conn, err := ctxDialer{ctx: ctx, d: transform.TargetDialer(true)}.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	defer conn.Close() //nolint:errcheck // test

	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("the read went through")
	}

	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the read outlived the cancel by %v", took)
	}

	// The guarded dialer refuses this host when private targets are off.
	if _, err := (ctxDialer{ctx: t.Context(), d: transform.TargetDialer(false)}).Dial("tcp", l.Addr().String()); err == nil {
		t.Error("the loopback was dialled")
	}
}
