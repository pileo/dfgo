package llm

import (
	"context"
	"io"
	"sync"
)

// StreamEventType identifies what kind of streaming event this is.
type StreamEventType string

const (
	EventContentStart StreamEventType = "content.start"
	EventContentDelta StreamEventType = "content.delta"
	EventContentStop  StreamEventType = "content.stop"
	EventResponseMeta StreamEventType = "response.meta" // id, model at stream start
	EventUsage        StreamEventType = "usage"          // token counts
	EventError        StreamEventType = "error"
)

// StreamEvent is a single event from a streaming LLM response.
type StreamEvent struct {
	Type         StreamEventType
	Index        int          // content block index (0-based)
	ContentKind  ContentKind  // text, tool_call, thinking (for start/delta/stop)
	Text         string       // delta text (for delta events)
	ToolCallID   string       // for tool_call content.start
	ToolName     string       // for tool_call content.start
	ResponseID   string       // for response.meta
	Model        string       // for response.meta
	Usage        *Usage       // for usage events
	FinishReason FinishReason // set on final usage event
	Err          error        // for error events
}

// Stream reads streaming events from an LLM provider and accumulates a final Response.
// Usage follows the scanner pattern:
//
//	stream, err := client.Stream(ctx, req)
//	for stream.Next() {
//	    evt := stream.Event()
//	    // handle evt
//	}
//	if err := stream.Err(); err != nil { ... }
//	resp := stream.Response() // fully accumulated
type Stream struct {
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan StreamEvent
	cur    StreamEvent // current event from last Next() call

	mu   sync.Mutex
	resp *Response     // accumulated by producer
	err  error         // terminal error
	body io.ReadCloser // underlying HTTP response body
	done bool          // true after ch is closed
}

// NewStream creates a Stream. The provider goroutine should send events via
// Send() and call Finish() when done. Finish() closes the channel.
func NewStream(ctx context.Context, body io.ReadCloser, bufSize int) *Stream {
	ctx, cancel := context.WithCancel(ctx)
	return &Stream{
		ctx:    ctx,
		cancel: cancel,
		ch:     make(chan StreamEvent, bufSize),
		body:   body,
	}
}

// Next advances to the next event. Returns false when the stream is done.
func (s *Stream) Next() bool {
	evt, ok := <-s.ch
	if !ok {
		s.mu.Lock()
		s.done = true
		s.mu.Unlock()
		return false
	}
	s.cur = evt
	return true
}

// Event returns the current event. Only valid after Next() returns true.
func (s *Stream) Event() StreamEvent {
	return s.cur
}

// Err returns the terminal error, if any. Only valid after Next() returns false.
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Response returns the fully accumulated response. Only valid after Next()
// returns false and Err() is nil.
func (s *Stream) Response() *Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resp
}

// Close cancels the stream, drains remaining events, and closes the body.
func (s *Stream) Close() error {
	s.cancel()
	// Drain remaining events so the producer goroutine can finish.
	for range s.ch {
	}
	s.mu.Lock()
	body := s.body
	s.done = true
	s.mu.Unlock()
	if body != nil {
		return body.Close()
	}
	return nil
}

// Body returns the underlying response body, for use by provider goroutines.
func (s *Stream) Body() io.ReadCloser {
	return s.body
}

// Send sends an event to the consumer. Returns false if the context is cancelled.
// Called by provider goroutines.
func (s *Stream) Send(evt StreamEvent) bool {
	select {
	case s.ch <- evt:
		return true
	case <-s.ctx.Done():
		return false
	}
}

// Finish sets the final response and error, then closes the channel.
// Called by provider goroutines when streaming is complete.
func (s *Stream) Finish(resp *Response, err error) {
	s.mu.Lock()
	s.resp = resp
	s.err = err
	s.mu.Unlock()
	close(s.ch)
}

// CompleteToStream wraps a Complete() response as a single-event stream.
// Used as a fallback when the provider doesn't support streaming.
func CompleteToStream(resp *Response, err error) *Stream {
	// Calculate buffer size: 1 meta + 3 per content part + 1 usage = good default.
	bufSize := 8
	if resp != nil {
		bufSize = 2 + len(resp.Message.Content)*3
	}
	st := &Stream{
		ch: make(chan StreamEvent, bufSize),
	}
	// No body to close, no context to cancel.
	st.ctx, st.cancel = context.WithCancel(context.Background())

	if err != nil {
		st.Finish(nil, err)
		return st
	}

	// Emit response metadata.
	st.ch <- StreamEvent{
		Type:       EventResponseMeta,
		ResponseID: resp.ID,
		Model:      resp.Model,
	}

	// Emit content as start/delta/stop for each content part.
	for i, part := range resp.Message.Content {
		switch part.Kind {
		case ContentText:
			st.ch <- StreamEvent{Type: EventContentStart, Index: i, ContentKind: ContentText}
			st.ch <- StreamEvent{Type: EventContentDelta, Index: i, ContentKind: ContentText, Text: part.Text}
			st.ch <- StreamEvent{Type: EventContentStop, Index: i, ContentKind: ContentText}
		case ContentToolCall:
			if part.ToolCall != nil {
				st.ch <- StreamEvent{
					Type: EventContentStart, Index: i, ContentKind: ContentToolCall,
					ToolCallID: part.ToolCall.ID, ToolName: part.ToolCall.Name,
				}
				st.ch <- StreamEvent{
					Type: EventContentDelta, Index: i, ContentKind: ContentToolCall,
					Text: string(part.ToolCall.Arguments),
				}
				st.ch <- StreamEvent{Type: EventContentStop, Index: i, ContentKind: ContentToolCall}
			}
		case ContentThinking:
			if part.Thinking != nil {
				st.ch <- StreamEvent{Type: EventContentStart, Index: i, ContentKind: ContentThinking}
				st.ch <- StreamEvent{Type: EventContentDelta, Index: i, ContentKind: ContentThinking, Text: part.Thinking.Text}
				st.ch <- StreamEvent{Type: EventContentStop, Index: i, ContentKind: ContentThinking}
			}
		}
	}

	// Emit usage.
	st.ch <- StreamEvent{
		Type:         EventUsage,
		Usage:        &resp.Usage,
		FinishReason: resp.FinishReason,
	}

	st.Finish(resp, nil)
	return st
}
