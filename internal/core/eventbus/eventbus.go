// Gosint, Copyright (c) 2026 YouSysAdmin

// Package eventbus fans sketch activity out to live event streams.
//
// Delivery is best effort: a subscriber that falls behind loses events
// rather than slowing a worker down, and the durable logs and scans
// tables stay the authority. On postgres a relay carries events between
// nodes, so a stream sees what a worker elsewhere did.
package eventbus

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Event types.
const (
	// TypeLog carries a logentry.Entry.
	TypeLog = "log"

	// TypeScan carries a scan whose status changed.
	TypeScan = "scan"

	// TypeGraph says a member changed the graph by hand (a GraphChange),
	// so the others reload.
	TypeGraph = "graph"
)

// GraphChange is what a graph event carries: who did what.
type GraphChange struct {
	By   string `json:"by"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Event is one thing that happened on a sketch.
type Event struct {
	Type     string    `json:"type"`
	SketchID string    `json:"sketch_id"`
	Data     any       `json:"data"`
	At       time.Time `json:"at"`
}

// Limits on live streams, so a browser opening tabs cannot exhaust the
// process.
const (
	MaxSubscribersPerSketch = 64
	MaxSubscribers          = 1024
)

// ErrTooManySubscribers refuses a stream past the limits.
var ErrTooManySubscribers = errors.New("too many live event streams")

const buffer = 256

// Subscription is one live stream's queue. C closes when the
// subscription or the bus closes.
type Subscription struct {
	C <-chan Event

	bus      *Bus
	id       uint64
	sketchID string
	ch       chan Event
}

// Close ends the subscription. Idempotent.
func (s *Subscription) Close() {
	if s == nil || s.bus == nil {
		return
	}

	s.bus.unsubscribe(s)
}

// Bus is the in-process fan-out, keyed by sketch.
type Bus struct {
	mu      sync.RWMutex
	nextID  uint64
	subs    map[string]map[uint64]*Subscription
	closed  bool
	dropped atomic.Uint64
	relay   atomic.Pointer[func(Event)]
}

// New builds an empty bus.
func New() *Bus {
	return &Bus{subs: map[string]map[uint64]*Subscription{}}
}

// SetRelay makes Publish also hand every event to send, which carries
// it to the other nodes.
func (b *Bus) SetRelay(send func(Event)) {
	b.relay.Store(&send)
}

// Subscribe opens a stream for one sketch.
func (b *Bus) Subscribe(sketchID string) (*Subscription, error) {
	ch := make(chan Event, buffer)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		close(ch)

		return &Subscription{C: ch}, nil
	}

	total := 0
	for _, m := range b.subs {
		total += len(m)
	}

	if len(b.subs[sketchID]) >= MaxSubscribersPerSketch || total >= MaxSubscribers {
		return nil, ErrTooManySubscribers
	}

	b.nextID++
	sub := &Subscription{C: ch, bus: b, id: b.nextID, sketchID: sketchID, ch: ch}
	if b.subs[sketchID] == nil {
		b.subs[sketchID] = map[uint64]*Subscription{}
	}

	b.subs[sketchID][sub.id] = sub

	return sub, nil
}

func (b *Bus) unsubscribe(s *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m := b.subs[s.sketchID]
	if _, ok := m[s.id]; !ok {
		return
	}

	delete(m, s.id)
	if len(m) == 0 {
		delete(b.subs, s.sketchID)
	}

	close(s.ch)
}

// Publish delivers e here and, through the relay, on the other nodes.
func (b *Bus) Publish(e Event) {
	if b == nil || e.SketchID == "" {
		return
	}

	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}

	b.Deliver(e)

	if send := b.relay.Load(); send != nil {
		(*send)(e)
	}
}

// Deliver hands e to this node's subscribers only, which is what a
// relay calls with an event from another node.
func (b *Bus) Deliver(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subs[e.SketchID] {
		select {
		case sub.ch <- e:
		default:
			b.dropped.Add(1)
		}
	}
}

// Close ends every subscription, so open streams finish on shutdown.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for id, m := range b.subs {
		for _, sub := range m {
			close(sub.ch)
		}

		delete(b.subs, id)
	}
}

// Dropped is how many events slow subscribers missed.
func (b *Bus) Dropped() uint64 { return b.dropped.Load() }
