package events

import (
	"sync"
	"testing"
	"time"
)

func TestEmitterEmitAndReceive(t *testing.T) {
	e := NewEmitter(16)
	defer e.Close()

	var mu sync.Mutex
	var received []Event

	e.On(func(evt Event) {
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
	})

	e.Emit(PipelineStarted, map[string]any{"run_id": "r1"})
	e.Emit(StageStarted, map[string]any{"node_id": "n1"})

	// Give the drain goroutine time to process.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != PipelineStarted {
		t.Fatalf("expected PipelineStarted, got %s", received[0].Type)
	}
	if received[1].Type != StageStarted {
		t.Fatalf("expected StageStarted, got %s", received[1].Type)
	}
	if received[0].Data["run_id"] != "r1" {
		t.Fatalf("expected run_id=r1, got %v", received[0].Data["run_id"])
	}
}

func TestEmitterCloseWaits(t *testing.T) {
	e := NewEmitter(16)
	var count int
	var mu sync.Mutex

	e.On(func(evt Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	for i := 0; i < 10; i++ {
		e.Emit(StageCompleted, nil)
	}

	e.Close()

	mu.Lock()
	defer mu.Unlock()
	if count != 10 {
		t.Fatalf("expected 10 events processed after Close, got %d", count)
	}
}

func TestEmitterDoubleClose(t *testing.T) {
	e := NewEmitter(16)
	e.Close()
	e.Close() // should not panic
}

func TestEmitterMultipleCallbacks(t *testing.T) {
	e := NewEmitter(16)
	defer e.Close()

	var mu sync.Mutex
	counts := [2]int{}

	e.On(func(evt Event) {
		mu.Lock()
		counts[0]++
		mu.Unlock()
	})
	e.On(func(evt Event) {
		mu.Lock()
		counts[1]++
		mu.Unlock()
	})

	e.Emit(CheckpointSaved, nil)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if counts[0] != 1 || counts[1] != 1 {
		t.Fatalf("both callbacks should receive event: %v", counts)
	}
}

func TestEventJSON(t *testing.T) {
	evt := Event{
		Type:      PipelineStarted,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Data:      map[string]any{"key": "val"},
	}
	b := evt.JSON()
	if len(b) == 0 {
		t.Fatal("JSON should return non-empty bytes")
	}
}

func TestEventTypes(t *testing.T) {
	// Verify all event type constants are distinct.
	types := []Type{
		PipelineStarted, PipelineCompleted, PipelineFailed,
		StageStarted, StageCompleted, StageFailed, StageRetrying,
		ParallelStarted, ParallelBranchStarted, ParallelBranchCompleted, ParallelCompleted,
		InterviewStarted, InterviewCompleted, InterviewTimeout,
		CheckpointSaved,
	}
	seen := make(map[Type]bool)
	for _, typ := range types {
		if seen[typ] {
			t.Fatalf("duplicate event type: %s", typ)
		}
		seen[typ] = true
	}
	if len(seen) != 15 {
		t.Fatalf("expected 15 event types, got %d", len(seen))
	}
}
