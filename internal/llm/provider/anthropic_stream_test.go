package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dfgo/internal/llm"
)

func anthropicSSEServer(events string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, events)
	}))
}

func TestAnthropicStreamText(t *testing.T) {
	events := `event: message_start
data: {"type":"message_start","message":{"id":"msg_001","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":25,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: ping
data: {"type":"ping"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}

`
	srv := anthropicSSEServer(events)
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	stream, err := a.CompleteStream(context.Background(), llm.Request{
		Model:    "claude-sonnet-4-20250514",
		Messages: []llm.Message{llm.TextMessage(llm.RoleUser, "Hi")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var evts []llm.StreamEvent
	for stream.Next() {
		evts = append(evts, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}

	// Verify event sequence.
	assertEventType(t, evts, 0, llm.EventResponseMeta)
	assertEventType(t, evts, 1, llm.EventContentStart)
	assertEventType(t, evts, 2, llm.EventContentDelta)
	assertEventType(t, evts, 3, llm.EventContentDelta)
	assertEventType(t, evts, 4, llm.EventContentStop)
	assertEventType(t, evts, 5, llm.EventUsage)

	// Check deltas.
	if evts[2].Text != "Hello" {
		t.Errorf("delta[0] = %q", evts[2].Text)
	}
	if evts[3].Text != " world" {
		t.Errorf("delta[1] = %q", evts[3].Text)
	}

	// Check accumulated response.
	resp := stream.Response()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Text() != "Hello world" {
		t.Errorf("response text = %q", resp.Text())
	}
	if resp.ID != "msg_001" {
		t.Errorf("response ID = %q", resp.ID)
	}
	if resp.FinishReason != llm.FinishStop {
		t.Errorf("finish reason = %q", resp.FinishReason)
	}
	if resp.Usage.InputTokens != 25 {
		t.Errorf("input tokens = %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 12 {
		t.Errorf("output tokens = %d", resp.Usage.OutputTokens)
	}
}

func TestAnthropicStreamToolUse(t *testing.T) {
	events := `event: message_start
data: {"type":"message_start","message":{"id":"msg_002","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":50}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me read that."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"read_file"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"main.go\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":30}}

event: message_stop
data: {"type":"message_stop"}

`
	srv := anthropicSSEServer(events)
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	stream, err := a.CompleteStream(context.Background(), llm.Request{
		Model: "claude-sonnet-4-20250514",
	})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}

	resp := stream.Response()
	if resp.FinishReason != llm.FinishToolUse {
		t.Errorf("finish = %q", resp.FinishReason)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d", len(calls))
	}
	if calls[0].Name != "read_file" {
		t.Errorf("tool name = %q", calls[0].Name)
	}
	if calls[0].ID != "toolu_01" {
		t.Errorf("tool ID = %q", calls[0].ID)
	}
	if string(calls[0].Arguments) != `{"path":"main.go"}` {
		t.Errorf("tool args = %s", calls[0].Arguments)
	}
}

func TestAnthropicStreamThinking(t *testing.T) {
	events := `event: message_start
data: {"type":"message_start","message":{"id":"msg_003","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":10}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","text":"sig123"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Here's my answer."}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":20}}

event: message_stop
data: {"type":"message_stop"}

`
	srv := anthropicSSEServer(events)
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	stream, err := a.CompleteStream(context.Background(), llm.Request{Model: "claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
	}

	resp := stream.Response()
	if len(resp.Message.Content) != 2 {
		t.Fatalf("content parts = %d, want 2", len(resp.Message.Content))
	}

	// First part: thinking.
	if resp.Message.Content[0].Kind != llm.ContentThinking {
		t.Errorf("part[0].Kind = %q", resp.Message.Content[0].Kind)
	}
	if resp.Message.Content[0].Thinking.Text != "Let me think..." {
		t.Errorf("thinking text = %q", resp.Message.Content[0].Thinking.Text)
	}
	if resp.Message.Content[0].Thinking.Signature != "sig123" {
		t.Errorf("signature = %q", resp.Message.Content[0].Thinking.Signature)
	}

	// Second part: text.
	if resp.Message.Content[1].Kind != llm.ContentText {
		t.Errorf("part[1].Kind = %q", resp.Message.Content[1].Kind)
	}
	if resp.Message.Content[1].Text != "Here's my answer." {
		t.Errorf("text = %q", resp.Message.Content[1].Text)
	}
}

func TestAnthropicStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"too many requests"}}`))
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	_, err := a.CompleteStream(context.Background(), llm.Request{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !llm.IsRetryable(err) {
		t.Error("429 should be retryable")
	}
}

func TestAnthropicStreamDisconnect(t *testing.T) {
	// Stream that ends mid-way without message_stop.
	events := `event: message_start
data: {"type":"message_start","message":{"id":"msg_disc","type":"message","role":"assistant","model":"test","usage":{"input_tokens":5}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}

`
	srv := anthropicSSEServer(events)
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	stream, err := a.CompleteStream(context.Background(), llm.Request{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
	}

	if stream.Err() == nil {
		t.Error("expected error from incomplete stream")
	}
}

func assertEventType(t *testing.T, evts []llm.StreamEvent, idx int, want llm.StreamEventType) {
	t.Helper()
	if idx >= len(evts) {
		t.Fatalf("event[%d]: index out of range (have %d events)", idx, len(evts))
	}
	if evts[idx].Type != want {
		t.Errorf("event[%d].Type = %q, want %q", idx, evts[idx].Type, want)
	}
}
