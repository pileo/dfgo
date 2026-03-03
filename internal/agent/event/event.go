// Package event provides a typed event system for the agent loop.
package event

import (
	"encoding/json"
	"sync"
	"time"
)

// Type identifies a specific event kind.
type Type string

const (
	SessionStart    Type = "session.start"
	SessionEnd      Type = "session.end"
	TurnStart       Type = "turn.start"
	TurnEnd         Type = "turn.end"
	LLMRequest      Type = "llm.request"
	LLMResponse     Type = "llm.response"
	LLMError        Type = "llm.error"
	ToolStart       Type = "tool.start"
	ToolEnd         Type = "tool.end"
	ToolError       Type = "tool.error"
	Steering        Type = "steering"
	LoopDetected    Type = "loop.detected"
	ContextTruncate Type = "context.truncate"
	LLMStreamStart  Type = "llm.stream.start" // stream opened (response_id, model)
	LLMChunk        Type = "llm.chunk"         // content delta (kind, text, index)
	LLMStreamEnd    Type = "llm.stream.end"    // stream done (finish_reason, tokens)
	Abort           Type = "abort"
)

// Event is a single event emitted during agent execution.
type Event struct {
	Type      Type
	Timestamp time.Time
	Data      map[string]any
}

// JSON returns the event as a JSON byte slice.
func (e Event) JSON() []byte {
	b, _ := json.Marshal(e)
	return b
}

// Callback receives events from the emitter.
type Callback func(Event)

// Emitter dispatches events to registered callbacks via a buffered channel.
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
		// Buffer full — drop event rather than block the agent loop.
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
