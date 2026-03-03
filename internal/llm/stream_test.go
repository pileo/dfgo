package llm

import (
	"context"
	"testing"
)

func TestCompleteToStreamText(t *testing.T) {
	resp := &Response{
		ID:           "resp-1",
		Model:        "test-model",
		Provider:     "test",
		Message:      TextMessage(RoleAssistant, "hello world"),
		FinishReason: FinishStop,
		Usage:        Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}

	stream := CompleteToStream(resp, nil)

	var events []StreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}

	// Should have: meta, content.start, content.delta, content.stop, usage.
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5", len(events))
	}

	if events[0].Type != EventResponseMeta {
		t.Errorf("events[0].Type = %q", events[0].Type)
	}
	if events[0].ResponseID != "resp-1" {
		t.Errorf("ResponseID = %q", events[0].ResponseID)
	}

	if events[1].Type != EventContentStart || events[1].ContentKind != ContentText {
		t.Errorf("events[1] = %+v", events[1])
	}
	if events[2].Type != EventContentDelta || events[2].Text != "hello world" {
		t.Errorf("events[2] = %+v", events[2])
	}
	if events[3].Type != EventContentStop {
		t.Errorf("events[3] = %+v", events[3])
	}

	if events[4].Type != EventUsage || events[4].FinishReason != FinishStop {
		t.Errorf("events[4] = %+v", events[4])
	}
	if events[4].Usage.InputTokens != 10 {
		t.Errorf("usage input = %d", events[4].Usage.InputTokens)
	}

	got := stream.Response()
	if got.Text() != "hello world" {
		t.Errorf("response text = %q", got.Text())
	}
}

func TestCompleteToStreamError(t *testing.T) {
	stream := CompleteToStream(nil, &SDKError{Message: "test error"})

	if stream.Next() {
		t.Error("expected no events on error")
	}
	if stream.Err() == nil {
		t.Error("expected error")
	}
	if stream.Response() != nil {
		t.Error("expected nil response on error")
	}
}

func TestCompleteToStreamToolCall(t *testing.T) {
	resp := &Response{
		ID:    "resp-2",
		Model: "test",
		Message: Message{
			Role: RoleAssistant,
			Content: []ContentPart{
				{Kind: ContentText, Text: "calling tool"},
				{Kind: ContentToolCall, ToolCall: &ToolCallData{
					ID: "tc1", Name: "shell", Arguments: []byte(`{"cmd":"ls"}`),
				}},
			},
		},
		FinishReason: FinishToolUse,
		Usage:        Usage{InputTokens: 5, OutputTokens: 10},
	}

	stream := CompleteToStream(resp, nil)

	var events []StreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}

	// meta + (start+delta+stop)*2 + usage = 1 + 6 + 1 = 8
	if len(events) != 8 {
		t.Fatalf("got %d events, want 8", len(events))
	}

	// Check tool call start has correct fields.
	toolStart := events[4]
	if toolStart.Type != EventContentStart || toolStart.ContentKind != ContentToolCall {
		t.Errorf("tool start = %+v", toolStart)
	}
	if toolStart.ToolCallID != "tc1" || toolStart.ToolName != "shell" {
		t.Errorf("tool start = %+v", toolStart)
	}
}

func TestStreamClose(t *testing.T) {
	ctx := context.Background()
	stream := NewStream(ctx, nil, 4)

	// Send some events in a goroutine.
	go func() {
		stream.Send(StreamEvent{Type: EventResponseMeta, ResponseID: "test"})
		stream.Send(StreamEvent{Type: EventContentDelta, Text: "hello"})
		// Simulate a long stream that we'll close early.
		for i := 0; i < 100; i++ {
			if !stream.Send(StreamEvent{Type: EventContentDelta, Text: "more"}) {
				break
			}
		}
		stream.Finish(&Response{ID: "test"}, nil)
	}()

	// Read one event then close.
	if !stream.Next() {
		t.Fatal("expected at least one event")
	}

	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}
