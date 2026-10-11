// Gosint, Copyright (c) 2026 YouSysAdmin

package eventbus_test

import (
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
)

func TestAnEventReachesOnlyItsSketch(t *testing.T) {
	b := eventbus.New()
	a, _ := b.Subscribe("a")
	other, _ := b.Subscribe("b")

	defer a.Close()
	defer other.Close()

	b.Publish(eventbus.Event{Type: eventbus.TypeLog, SketchID: "a"})

	select {
	case e := <-a.C:
		if e.At.IsZero() {
			t.Error("event has no time")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber of a got nothing")
	}

	select {
	case e := <-other.C:
		t.Fatalf("subscriber of b got %+v", e)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAFullSubscriberDropsInsteadOfBlocking(t *testing.T) {
	b := eventbus.New()
	sub, _ := b.Subscribe("s")
	defer sub.Close()

	done := make(chan struct{})
	go func() {
		for range 1000 {
			b.Publish(eventbus.Event{Type: eventbus.TypeLog, SketchID: "s"})
		}

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}

	if b.Dropped() == 0 {
		t.Error("nothing reported as dropped")
	}
}

func TestTheRelaySeesPublishButNotDeliver(t *testing.T) {
	b := eventbus.New()
	var relayed []string
	b.SetRelay(func(e eventbus.Event) { relayed = append(relayed, e.Type) })

	b.Publish(eventbus.Event{Type: "local", SketchID: "s"})
	b.Deliver(eventbus.Event{Type: "remote", SketchID: "s"})

	if len(relayed) != 1 || relayed[0] != "local" {
		t.Errorf("relayed %v, want only the local event", relayed)
	}
}

func TestCloseEndsStreams(t *testing.T) {
	b := eventbus.New()
	sub, _ := b.Subscribe("s")
	b.Close()
	b.Close()

	if _, ok := <-sub.C; ok {
		t.Error("channel still open after Close")
	}

	sub.Close()

	late, _ := b.Subscribe("s")
	if _, ok := <-late.C; ok {
		t.Error("a subscription after Close is open")
	}
}

// TestMain fails the package when a test leaves a goroutine behind: a
// lease renewer or a stream that outlives its run would keep touching
// rows another node owns.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
