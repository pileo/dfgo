// Package events defines the observability event types for Attractor pipelines.
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// Type identifies a pipeline event kind.
type Type string

const (
	PipelineStarted  Type = "pipeline.started"
	PipelineCompleted Type = "pipeline.completed"
	PipelineFailed   Type = "pipeline.failed"

	StageStarted  Type = "stage.started"
	StageCompleted Type = "stage.completed"
	StageFailed   Type = "stage.failed"
	StageRetrying Type = "stage.retrying"

	ParallelStarted         Type = "parallel.started"
	ParallelBranchStarted   Type = "parallel.branch.started"
	ParallelBranchCompleted Type = "parallel.branch.completed"
	ParallelCompleted       Type = "parallel.completed"

	InterviewStarted   Type = "interview.started"
	InterviewCompleted Type = "interview.completed"
	InterviewTimeout   Type = "interview.timeout"

	CheckpointSaved Type = "checkpoint.saved"
)

// Event is a single observability event emitted during pipeline execution.
type Event struct {
	Type      Type           `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// JSON returns the event serialized as JSON.
func (e Event) JSON() []byte {
	b, _ := json.Marshal(e)
	return b
}

// Callback receives events from the emitter.
type Callback func(Event)

// Emitter dispatches pipeline events to registered callbacks via a buffered channel.
type Emitter struct {
	mu        sync.RWMutex
	callbacks []Callback
	ch        chan Event
	done      chan struct{}
	closed    bool
}

// NewEmitter creates a new Emitter with a buffered channel of the given capacity.
func NewEmitter(bufSize int) *Emitter {
	if bufSize <= 0 {
		bufSize = 256
	}
	e := &Emitter{
		ch:   make(chan Event, bufSize),
		done: make(chan struct{}),
	}
	go e.drain()
	return e
}

// On registers a callback for all events.
func (e *Emitter) On(cb Callback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callbacks = append(e.callbacks, cb)
}

// Emit sends an event. Non-blocking: drops if buffer is full.
func (e *Emitter) Emit(typ Type, data map[string]any) {
	evt := Event{
		Type:      typ,
		Timestamp: time.Now(),
		Data:      data,
	}
	select {
	case e.ch <- evt:
	default:
	}
}

// Close stops the emitter and waits for pending events to drain.
func (e *Emitter) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()
	close(e.ch)
	<-e.done
}

func (e *Emitter) drain() {
	defer close(e.done)
	for evt := range e.ch {
		e.mu.RLock()
		cbs := make([]Callback, len(e.callbacks))
		copy(cbs, e.callbacks)
		e.mu.RUnlock()
		for _, cb := range cbs {
			cb(evt)
		}
	}
}
