// Package sse provides a fan-out event broadcaster with replay support for SSE streaming.
package sse

import (
	"context"
	"sync"
	"sync/atomic"

	"dfgo/internal/attractor/events"
)

const (
	// MaxHistory is the maximum number of events kept for replay.
	MaxHistory = 1024
	// subBufSize is the channel buffer size for each subscriber.
	subBufSize = 64
)

// Broadcaster fans out events to multiple SSE subscribers with replay support.
type Broadcaster struct {
	mu      sync.RWMutex
	history []events.Event
	subs    map[uint64]*Subscription
	nextID  atomic.Uint64
	closed  bool
}

// Subscription represents a single SSE client's event stream.
type Subscription struct {
	C      <-chan events.Event // Consumer reads from this.
	ch     chan events.Event   // Internal write end.
	id     uint64
	cancel context.CancelFunc
}

// NewBroadcaster creates a new Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[uint64]*Subscription),
	}
}

// Publish adds an event to history and fans it out to all subscribers.
// Safe for use as an events.Callback.
func (b *Broadcaster) Publish(evt events.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}

	// Append to history, trim to ring buffer size.
	b.history = append(b.history, evt)
	if len(b.history) > MaxHistory {
		b.history = b.history[len(b.history)-MaxHistory:]
	}

	// Non-blocking fan-out to all subscribers.
	for _, sub := range b.subs {
		select {
		case sub.ch <- evt:
		default:
			// Drop event for slow client.
		}
	}
}

// Subscribe creates a new subscription. All history events are replayed into the
// channel before live events begin. The channel closes when the broadcaster closes
// or ctx is canceled.
func (b *Broadcaster) Subscribe(ctx context.Context) *Subscription {
	ch := make(chan events.Event, subBufSize)
	ctx, cancel := context.WithCancel(ctx)
	id := b.nextID.Add(1)

	sub := &Subscription{
		C:      ch,
		ch:     ch,
		id:     id,
		cancel: cancel,
	}

	b.mu.Lock()
	// Replay history while holding the lock so no events are missed
	// between replay and registration.
	for _, evt := range b.history {
		select {
		case ch <- evt:
		default:
		}
	}
	if b.closed {
		close(ch)
		b.mu.Unlock()
		return sub
	}
	b.subs[id] = sub
	b.mu.Unlock()

	// Unregister when context is canceled.
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(ch)
		}
	}()

	return sub
}

// History returns a copy of all buffered events.
func (b *Broadcaster) History() []events.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]events.Event, len(b.history))
	copy(out, b.history)
	return out
}

// Close closes all subscriber channels. Called when the run finishes.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, sub := range b.subs {
		sub.cancel()
		close(sub.ch)
		delete(b.subs, id)
	}
}
