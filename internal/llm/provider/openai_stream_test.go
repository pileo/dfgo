package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dfgo/internal/llm"
)

func openaiSSEServer(events string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, events)
	}))
}

func TestOpenAIStreamText(t *testing.T) {
	events := `event: response.created
data: {"id":"resp_001","object":"response","model":"gpt-4o","status":"in_progress"}

event: response.output_item.added
data: {"output_index":0,"item":{"type":"message","id":"msg_001"}}

event: response.content_part.added
data: {"output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}

event: response.output_text.delta
data: {"output_index":0,"content_index":0,"delta":"Hello"}

event: response.output_text.delta
data: {"output_index":0,"content_index":0,"delta":" world"}

event: response.output_item.done
data: {"item":{"type":"message","id":"msg_001","content":[{"type":"output_text","text":"Hello world"}]}}

event: response.completed
data: {"id":"resp_001","model":"gpt-4o","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"output_tokens_details":{"reasoning_tokens":0}}}

`
	srv := openaiSSEServer(events)
	defer srv.Close()

	o := NewOpenAI(func(o *OpenAI) {
		o.APIKey = "test-key"
		o.BaseURL = srv.URL
	})

	stream, err := o.CompleteStream(context.Background(), llm.Request{
		Model:    "gpt-4o",
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

	// Check response meta.
	if len(evts) == 0 {
		t.Fatal("no events")
	}
	if evts[0].Type != llm.EventResponseMeta {
		t.Errorf("first event = %q", evts[0].Type)
	}
	if evts[0].ResponseID != "resp_001" {
		t.Errorf("response ID = %q", evts[0].ResponseID)
	}

	// Verify text deltas exist.
	var text string
	for _, e := range evts {
		if e.Type == llm.EventContentDelta && e.ContentKind == llm.ContentText {
			text += e.Text
		}
	}
	if text != "Hello world" {
		t.Errorf("accumulated text = %q", text)
	}

	// Check accumulated response.
	resp := stream.Response()
	if resp.Text() != "Hello world" {
		t.Errorf("response text = %q", resp.Text())
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("input tokens = %d", resp.Usage.InputTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("total tokens = %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIStreamFunctionCall(t *testing.T) {
	events := `event: response.created
data: {"id":"resp_002","model":"gpt-4o"}

event: response.output_item.added
data: {"output_index":0,"item":{"type":"function_call","id":"fc_001","name":"shell","call_id":"call_001"}}

event: response.function_call_arguments.delta
data: {"output_index":0,"delta":"{\"command\":"}

event: response.function_call_arguments.delta
data: {"output_index":0,"delta":"\"ls\"}"}

event: response.output_item.done
data: {"item":{"type":"function_call","id":"fc_001","name":"shell","call_id":"call_001","arguments":"{\"command\":\"ls\"}"}}

event: response.completed
data: {"id":"resp_002","model":"gpt-4o","usage":{"input_tokens":20,"output_tokens":10,"total_tokens":30}}

`
	srv := openaiSSEServer(events)
	defer srv.Close()

	o := NewOpenAI(func(o *OpenAI) {
		o.APIKey = "test-key"
		o.BaseURL = srv.URL
	})

	stream, err := o.CompleteStream(context.Background(), llm.Request{Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify tool-related events.
	var hasToolStart, hasToolDelta bool
	for stream.Next() {
		e := stream.Event()
		if e.Type == llm.EventContentStart && e.ContentKind == llm.ContentToolCall {
			hasToolStart = true
			if e.ToolName != "shell" {
				t.Errorf("tool name = %q", e.ToolName)
			}
		}
		if e.Type == llm.EventContentDelta && e.ContentKind == llm.ContentToolCall {
			hasToolDelta = true
		}
	}
	if !hasToolStart {
		t.Error("missing tool content.start")
	}
	if !hasToolDelta {
		t.Error("missing tool content.delta")
	}

	resp := stream.Response()
	if resp.FinishReason != llm.FinishToolUse {
		t.Errorf("finish = %q", resp.FinishReason)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d", len(calls))
	}
	if calls[0].Name != "shell" {
		t.Errorf("tool name = %q", calls[0].Name)
	}
}

func TestOpenAIStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer srv.Close()

	o := NewOpenAI(func(o *OpenAI) {
		o.APIKey = "test-key"
		o.BaseURL = srv.URL
	})

	_, err := o.CompleteStream(context.Background(), llm.Request{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}
