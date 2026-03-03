package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dfgo/internal/llm"
)

func geminiSSEServer(events string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, events)
	}))
}

func TestGeminiStreamText(t *testing.T) {
	events := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":12}}

data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hello world"}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}

`
	srv := geminiSSEServer(events)
	defer srv.Close()

	g := NewGemini(func(g *Gemini) {
		g.APIKey = "test-key"
		g.BaseURL = srv.URL
	})

	stream, err := g.CompleteStream(context.Background(), llm.Request{
		Model:    "gemini-2.0-flash",
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

	// Should have events including meta, content start/delta/stop, usage.
	if len(evts) == 0 {
		t.Fatal("no events")
	}

	// Check that we got text deltas.
	var text string
	for _, e := range evts {
		if e.Type == llm.EventContentDelta && e.ContentKind == llm.ContentText {
			text += e.Text
		}
	}
	// Gemini sends cumulative text, so we get "Hello" + "Hello world".
	// The implementation emits deltas from each chunk, so we get both.
	if text == "" {
		t.Error("no text deltas received")
	}

	resp := stream.Response()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.FinishReason != llm.FinishStop {
		t.Errorf("finish = %q", resp.FinishReason)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("input tokens = %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("output tokens = %d", resp.Usage.OutputTokens)
	}
}

func TestGeminiStreamFunctionCall(t *testing.T) {
	events := `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"main.go"}}}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":15,"candidatesTokenCount":8,"totalTokenCount":23}}

`
	srv := geminiSSEServer(events)
	defer srv.Close()

	g := NewGemini(func(g *Gemini) {
		g.APIKey = "test-key"
		g.BaseURL = srv.URL
	})

	stream, err := g.CompleteStream(context.Background(), llm.Request{Model: "gemini-2.0-flash"})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
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
	if calls[0].ID == "" {
		t.Error("expected synthetic UUID")
	}
}

func TestGeminiStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	g := NewGemini(func(g *Gemini) {
		g.APIKey = "test-key"
		g.BaseURL = srv.URL
	})

	_, err := g.CompleteStream(context.Background(), llm.Request{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}
