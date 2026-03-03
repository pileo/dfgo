package event

import (
	"sync"
	"testing"
	"time"
)

func TestEmitterBasic(t *testing.T) {
	e := NewEmitter(16)
	defer e.Close()

	var mu sync.Mutex
	var received []Event
	e.On(func(evt Event) {
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
	})

	e.Emit(SessionStart, map[string]any{"session_id": "s1"})
	e.Emit(TurnStart, map[string]any{"turn": 1})

	// Give drain goroutine time to process.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("received %d events, want 2", len(received))
	}
	if received[0].Type != SessionStart {
		t.Errorf("event[0].Type = %q", received[0].Type)
	}
	if received[1].Type != TurnStart {
		t.Errorf("event[1].Type = %q", received[1].Type)
	}
}

func TestEmitterMultipleCallbacks(t *testing.T) {
	e := NewEmitter(16)
	defer e.Close()

	var count1, count2 int
	var mu sync.Mutex
	e.On(func(evt Event) { mu.Lock(); count1++; mu.Unlock() })
	e.On(func(evt Event) { mu.Lock(); count2++; mu.Unlock() })

	e.Emit(ToolStart, nil)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count1 != 1 || count2 != 1 {
		t.Errorf("count1=%d, count2=%d, want 1,1", count1, count2)
	}
}

func TestEmitterCloseWaits(t *testing.T) {
	e := NewEmitter(16)

	var count int
	var mu sync.Mutex
	e.On(func(evt Event) { mu.Lock(); count++; mu.Unlock() })

	for i := 0; i < 10; i++ {
		e.Emit(TurnStart, nil)
	}
	e.Close()

	mu.Lock()
	defer mu.Unlock()
	if count != 10 {
		t.Errorf("count = %d, want 10 (all events drained)", count)
	}
}

func TestEventJSON(t *testing.T) {
	evt := Event{
		Type:      ToolStart,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Data:      map[string]any{"tool": "shell"},
	}
	b := evt.JSON()
	if len(b) == 0 {
		t.Fatal("empty JSON")
	}
}
