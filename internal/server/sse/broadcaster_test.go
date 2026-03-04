package sse

import (
	"context"
	"testing"
	"time"

	"dfgo/internal/attractor/events"
)

func testEvent(typ events.Type) events.Event {
	return events.Event{Type: typ, Timestamp: time.Now(), Data: map[string]any{"test": true}}
}

func TestPublishAndSubscribe(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := b.Subscribe(ctx)

	b.Publish(testEvent(events.StageStarted))
	b.Publish(testEvent(events.StageCompleted))

	got := 0
	timeout := time.After(time.Second)
	for got < 2 {
		select {
		case <-sub.C:
			got++
		case <-timeout:
			t.Fatalf("timed out waiting for events, got %d", got)
		}
	}
}

func TestReplay(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	// Publish before subscribing.
	b.Publish(testEvent(events.PipelineStarted))
	b.Publish(testEvent(events.StageStarted))
	b.Publish(testEvent(events.StageCompleted))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := b.Subscribe(ctx)

	// Should receive all 3 replayed events.
	got := 0
	timeout := time.After(time.Second)
	for got < 3 {
		select {
		case <-sub.C:
			got++
		case <-timeout:
			t.Fatalf("timed out waiting for replay events, got %d", got)
		}
	}
}

func TestHistory(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	b.Publish(testEvent(events.PipelineStarted))
	b.Publish(testEvent(events.StageStarted))

	h := b.History()
	if len(h) != 2 {
		t.Fatalf("expected 2 history events, got %d", len(h))
	}
	if h[0].Type != events.PipelineStarted {
		t.Errorf("expected first event PipelineStarted, got %s", h[0].Type)
	}
}

func TestHistoryRingBuffer(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	for i := 0; i < MaxHistory+100; i++ {
		b.Publish(testEvent(events.StageStarted))
	}

	h := b.History()
	if len(h) != MaxHistory {
		t.Fatalf("expected history capped at %d, got %d", MaxHistory, len(h))
	}
}

func TestCloseEndsSubscription(t *testing.T) {
	b := NewBroadcaster()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := b.Subscribe(ctx)
	b.Close()

	// Channel should be closed.
	select {
	case _, ok := <-sub.C:
		if ok {
			// Might drain a buffered event, keep reading.
			for range sub.C {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("subscription channel not closed after broadcaster Close")
	}
}

func TestContextCancelUnsubscribes(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sub := b.Subscribe(ctx)
	cancel()

	// Channel should close.
	select {
	case _, ok := <-sub.C:
		if ok {
			for range sub.C {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("subscription channel not closed after context cancel")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	sub1 := b.Subscribe(ctx1)
	sub2 := b.Subscribe(ctx2)

	b.Publish(testEvent(events.StageStarted))

	timeout := time.After(time.Second)
	for _, sub := range []*Subscription{sub1, sub2} {
		select {
		case evt := <-sub.C:
			if evt.Type != events.StageStarted {
				t.Errorf("expected StageStarted, got %s", evt.Type)
			}
		case <-timeout:
			t.Fatal("timed out waiting for event on subscriber")
		}
	}
}
