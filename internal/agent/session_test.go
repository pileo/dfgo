package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"dfgo/internal/agent/event"
	"dfgo/internal/agent/execenv"
	"dfgo/internal/agent/message"
	"dfgo/internal/agent/profile"
	"dfgo/internal/llm"
)

// mockLLMProvider implements llm.ProviderAdapter for testing.
type mockLLMProvider struct {
	mu        sync.Mutex
	responses []*llm.Response
	callIndex int
}

func (m *mockLLMProvider) Name() string { return "mock" }

func (m *mockLLMProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIndex >= len(m.responses) {
		return &llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "done"),
			FinishReason: llm.FinishStop,
		}, nil
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func newTestClient(responses ...*llm.Response) *llm.Client {
	prov := &mockLLMProvider{responses: responses}
	return llm.NewClient(llm.WithProvider(prov))
}

func TestSessionNaturalExit(t *testing.T) {
	client := newTestClient(&llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, "Hello! I can help you."),
		FinishReason: llm.FinishStop,
		Usage:        llm.Usage{InputTokens: 100, OutputTokens: 20},
	})

	env := execenv.NewLocal(t.TempDir())
	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     env,
		Model:   "test-model",
	}
	session := NewSession(cfg)

	result := session.Run(context.Background(), "Hello")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Aborted {
		t.Error("should not be aborted")
	}
	if result.FinalText != "Hello! I can help you." {
		t.Errorf("final text = %q", result.FinalText)
	}
	if result.Rounds != 1 {
		t.Errorf("rounds = %d, want 1", result.Rounds)
	}
	if result.TotalUsage.InputTokens != 100 {
		t.Errorf("input tokens = %d", result.TotalUsage.InputTokens)
	}
}

func TestSessionToolUse(t *testing.T) {
	dir := t.TempDir()
	env := execenv.NewLocal(dir)
	// Write a test file for the agent to read.
	env.WriteFile(context.Background(), "test.txt", []byte("file contents"), 0644)

	client := newTestClient(
		// First response: tool call.
		&llm.Response{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				Content: []llm.ContentPart{
					{Kind: llm.ContentText, Text: "I'll read the file."},
					{Kind: llm.ContentToolCall, ToolCall: &llm.ToolCallData{
						ID:        "tc_1",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"test.txt"}`),
					}},
				},
			},
			FinishReason: llm.FinishToolUse,
			Usage:        llm.Usage{InputTokens: 50, OutputTokens: 30},
		},
		// Second response: final answer after seeing tool result.
		&llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "The file contains: file contents"),
			FinishReason: llm.FinishStop,
			Usage:        llm.Usage{InputTokens: 80, OutputTokens: 15},
		},
	)

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     env,
		Model:   "test-model",
	}
	session := NewSession(cfg)

	result := session.Run(context.Background(), "Read test.txt")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Rounds != 2 {
		t.Errorf("rounds = %d, want 2", result.Rounds)
	}
	if result.TotalUsage.InputTokens != 130 {
		t.Errorf("input tokens = %d, want 130", result.TotalUsage.InputTokens)
	}
	if result.FinalText != "The file contains: file contents" {
		t.Errorf("final text = %q", result.FinalText)
	}
}

func TestSessionMaxRounds(t *testing.T) {
	// Create a provider that always returns tool calls.
	toolCallResp := &llm.Response{
		Message: llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.ContentPart{
				{Kind: llm.ContentToolCall, ToolCall: &llm.ToolCallData{
					ID:        "tc_1",
					Name:      "shell",
					Arguments: json.RawMessage(`{"command":"echo hi"}`),
				}},
			},
		},
		FinishReason: llm.FinishToolUse,
	}
	responses := make([]*llm.Response, 10)
	for i := range responses {
		responses[i] = toolCallResp
	}
	client := newTestClient(responses...)

	cfg := Config{
		Client:    client,
		Profile:   profile.Anthropic{},
		Env:       execenv.NewLocal(t.TempDir()),
		Model:     "test-model",
		MaxRounds: 3,
	}
	session := NewSession(cfg)

	result := session.Run(context.Background(), "do something")
	if !result.Aborted {
		t.Error("expected abort due to max rounds")
	}
	if result.Rounds > 3 {
		t.Errorf("rounds = %d, expected <= 3", result.Rounds)
	}
}

func TestSessionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	client := newTestClient()
	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)

	result := session.Run(ctx, "hello")
	if !result.Aborted {
		t.Error("expected abort due to context cancellation")
	}
}

func TestSessionEvents(t *testing.T) {
	client := newTestClient(&llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, "done"),
		FinishReason: llm.FinishStop,
	})

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)

	var mu sync.Mutex
	var events []event.Type
	session.OnEvent(func(e event.Event) {
		mu.Lock()
		events = append(events, e.Type)
		mu.Unlock()
	})

	session.Run(context.Background(), "hi")

	mu.Lock()
	defer mu.Unlock()
	// Should have: session.start, turn.start, llm.request, llm.response, turn.end, session.end
	expected := map[event.Type]bool{
		event.SessionStart: true,
		event.SessionEnd:   true,
		event.TurnStart:    true,
		event.LLMRequest:   true,
		event.LLMResponse:  true,
		event.TurnEnd:      true,
	}
	for _, e := range events {
		delete(expected, e)
	}
	for e := range expected {
		t.Errorf("missing event: %s", e)
	}
}

func TestSessionSteering(t *testing.T) {
	callCount := 0
	prov := &customTestProvider{fn: func(ctx context.Context, req llm.Request) (*llm.Response, error) {
		callCount++
		if callCount == 1 {
			// First call: return tool call.
			return &llm.Response{
				Message: llm.Message{
					Role: llm.RoleAssistant,
					Content: []llm.ContentPart{
						{Kind: llm.ContentToolCall, ToolCall: &llm.ToolCallData{
							ID:        "tc_1",
							Name:      "shell",
							Arguments: json.RawMessage(`{"command":"echo test"}`),
						}},
					},
				},
				FinishReason: llm.FinishToolUse,
			}, nil
		}
		// Check that steering message was included.
		for _, m := range req.Messages {
			if text := m.Text(); text != "" {
				if len(text) > 8 && text[:8] == "[SYSTEM]" {
					// Found steering message.
					return &llm.Response{
						Message:      llm.TextMessage(llm.RoleAssistant, "acknowledged steering"),
						FinishReason: llm.FinishStop,
					}, nil
				}
			}
		}
		return &llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "no steering found"),
			FinishReason: llm.FinishStop,
		}, nil
	}}

	client := llm.NewClient(llm.WithProvider(prov))
	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)
	session.Steer("Focus on testing")

	result := session.Run(context.Background(), "work on this")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.FinalText != "acknowledged steering" {
		t.Errorf("final text = %q, expected steering acknowledgment", result.FinalText)
	}
}

func TestSessionToolError(t *testing.T) {
	client := newTestClient(
		// Tool call to read non-existent file.
		&llm.Response{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				Content: []llm.ContentPart{
					{Kind: llm.ContentToolCall, ToolCall: &llm.ToolCallData{
						ID:        "tc_1",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"nonexistent.txt"}`),
					}},
				},
			},
			FinishReason: llm.FinishToolUse,
		},
		// Model handles the error gracefully.
		&llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "The file doesn't exist."),
			FinishReason: llm.FinishStop,
		},
	)

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)

	result := session.Run(context.Background(), "read the file")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	// Tool error should be passed to model as is_error, not crash the session.
	if result.FinalText != "The file doesn't exist." {
		t.Errorf("final text = %q", result.FinalText)
	}
}

func TestAgentToLLMMessageConversions(t *testing.T) {
	// User message.
	m := agentToLLMMessage(message.UserMessage("hello"))
	if m.Role != llm.RoleUser {
		t.Errorf("role = %q", m.Role)
	}
	if m.Text() != "hello" {
		t.Errorf("text = %q", m.Text())
	}

	// Tool result message.
	m = agentToLLMMessage(message.ToolResultMessage("tc_1", "result", false))
	if m.Role != llm.RoleTool {
		t.Errorf("role = %q", m.Role)
	}
	if m.ToolCallID != "tc_1" {
		t.Errorf("tool call id = %q", m.ToolCallID)
	}

	// Steering message.
	m = agentToLLMMessage(message.SteeringMessage("focus"))
	if m.Role != llm.RoleUser {
		t.Errorf("role = %q", m.Role)
	}
	if m.Text() != "[SYSTEM] focus" {
		t.Errorf("text = %q", m.Text())
	}
}

func TestSessionStreamingNaturalExit(t *testing.T) {
	client := newTestClient(&llm.Response{
		ID:           "resp-stream-1",
		Model:        "test-model",
		Message:      llm.TextMessage(llm.RoleAssistant, "streamed response"),
		FinishReason: llm.FinishStop,
		Usage:        llm.Usage{InputTokens: 50, OutputTokens: 10},
	})

	cfg := Config{
		Client:    client,
		Profile:   profile.Anthropic{},
		Env:       execenv.NewLocal(t.TempDir()),
		Model:     "test-model",
		Streaming: true,
	}
	session := NewSession(cfg)

	var mu sync.Mutex
	var events []event.Type
	session.OnEvent(func(e event.Event) {
		mu.Lock()
		events = append(events, e.Type)
		mu.Unlock()
	})

	result := session.Run(context.Background(), "Hello")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Aborted {
		t.Error("should not be aborted")
	}
	if result.FinalText != "streamed response" {
		t.Errorf("final text = %q", result.FinalText)
	}
	if result.TotalUsage.InputTokens != 50 {
		t.Errorf("input tokens = %d", result.TotalUsage.InputTokens)
	}

	// Verify streaming-specific events were emitted.
	mu.Lock()
	defer mu.Unlock()
	hasStreamStart := false
	hasChunk := false
	hasStreamEnd := false
	for _, e := range events {
		switch e {
		case event.LLMStreamStart:
			hasStreamStart = true
		case event.LLMChunk:
			hasChunk = true
		case event.LLMStreamEnd:
			hasStreamEnd = true
		}
	}
	if !hasStreamStart {
		t.Error("missing llm.stream.start event")
	}
	if !hasChunk {
		t.Error("missing llm.chunk event")
	}
	if !hasStreamEnd {
		t.Error("missing llm.stream.end event")
	}
}

func TestSessionStreamingToolUse(t *testing.T) {
	dir := t.TempDir()
	env := execenv.NewLocal(dir)
	env.WriteFile(context.Background(), "test.txt", []byte("streaming file"), 0644)

	client := newTestClient(
		&llm.Response{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				Content: []llm.ContentPart{
					{Kind: llm.ContentText, Text: "Reading the file."},
					{Kind: llm.ContentToolCall, ToolCall: &llm.ToolCallData{
						ID:        "tc_1",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"test.txt"}`),
					}},
				},
			},
			FinishReason: llm.FinishToolUse,
			Usage:        llm.Usage{InputTokens: 40, OutputTokens: 20},
		},
		&llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "File says: streaming file"),
			FinishReason: llm.FinishStop,
			Usage:        llm.Usage{InputTokens: 60, OutputTokens: 10},
		},
	)

	cfg := Config{
		Client:    client,
		Profile:   profile.Anthropic{},
		Env:       env,
		Model:     "test-model",
		Streaming: true,
	}
	session := NewSession(cfg)

	result := session.Run(context.Background(), "Read test.txt")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Rounds != 2 {
		t.Errorf("rounds = %d, want 2", result.Rounds)
	}
	if result.FinalText != "File says: streaming file" {
		t.Errorf("final text = %q", result.FinalText)
	}
	if result.TotalUsage.InputTokens != 100 {
		t.Errorf("input tokens = %d, want 100", result.TotalUsage.InputTokens)
	}
}

func TestSessionStreamingWithNativeProvider(t *testing.T) {
	// Test with a provider that implements StreamingProvider directly.
	streamUsed := false
	resp := &llm.Response{
		ID:           "resp-native",
		Model:        "test",
		Message:      llm.TextMessage(llm.RoleAssistant, "native stream"),
		FinishReason: llm.FinishStop,
		Usage:        llm.Usage{InputTokens: 10, OutputTokens: 5},
	}

	prov := &streamingTestProvider{
		resp: resp,
		onStream: func() {
			streamUsed = true
		},
	}
	client := llm.NewClient(llm.WithProvider(prov))

	cfg := Config{
		Client:    client,
		Profile:   profile.Anthropic{},
		Env:       execenv.NewLocal(t.TempDir()),
		Model:     "test",
		Streaming: true,
	}
	session := NewSession(cfg)

	result := session.Run(context.Background(), "hello")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !streamUsed {
		t.Error("expected native CompleteStream to be used")
	}
	if result.FinalText != "native stream" {
		t.Errorf("final text = %q", result.FinalText)
	}
}

// streamingTestProvider implements both ProviderAdapter and StreamingProvider.
type streamingTestProvider struct {
	resp     *llm.Response
	onStream func()
}

func (p *streamingTestProvider) Name() string { return "mock" }
func (p *streamingTestProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	return p.resp, nil
}
func (p *streamingTestProvider) CompleteStream(_ context.Context, _ llm.Request) (*llm.Stream, error) {
	if p.onStream != nil {
		p.onStream()
	}
	return llm.CompleteToStream(p.resp, nil), nil
}

// customTestProvider allows inline function-based provider mocking.
type customTestProvider struct {
	fn func(context.Context, llm.Request) (*llm.Response, error)
}

func (p *customTestProvider) Name() string { return "mock" }
func (p *customTestProvider) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	return p.fn(ctx, req)
}

func TestSessionFollowUp(t *testing.T) {
	callCount := 0
	prov := &customTestProvider{fn: func(ctx context.Context, req llm.Request) (*llm.Response, error) {
		callCount++
		switch callCount {
		case 1:
			// First call: answer the initial user message.
			return &llm.Response{
				Message:      llm.TextMessage(llm.RoleAssistant, "first answer"),
				FinishReason: llm.FinishStop,
				Usage:        llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		case 2:
			// Second call: answer the follow-up message.
			// Verify the follow-up content is in the messages.
			found := false
			for _, m := range req.Messages {
				if t := m.Text(); t == "follow up task" {
					found = true
				}
			}
			if !found {
				return &llm.Response{
					Message:      llm.TextMessage(llm.RoleAssistant, "no followup found"),
					FinishReason: llm.FinishStop,
				}, nil
			}
			return &llm.Response{
				Message:      llm.TextMessage(llm.RoleAssistant, "follow-up done"),
				FinishReason: llm.FinishStop,
				Usage:        llm.Usage{InputTokens: 20, OutputTokens: 10},
			}, nil
		default:
			return &llm.Response{
				Message:      llm.TextMessage(llm.RoleAssistant, "unexpected call"),
				FinishReason: llm.FinishStop,
			}, nil
		}
	}}

	client := llm.NewClient(llm.WithProvider(prov))
	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)

	// Queue a follow-up before running.
	session.FollowUp("follow up task")

	result := session.Run(context.Background(), "initial task")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	// The session should have processed both the initial message and the follow-up.
	if callCount != 2 {
		t.Errorf("LLM call count = %d, want 2", callCount)
	}
	if result.Rounds != 2 {
		t.Errorf("rounds = %d, want 2", result.Rounds)
	}
	// The final text should be from the follow-up response.
	if result.FinalText != "follow-up done" {
		t.Errorf("final text = %q, want %q", result.FinalText, "follow-up done")
	}
	// Usage should be accumulated.
	if result.TotalUsage.InputTokens != 30 {
		t.Errorf("input tokens = %d, want 30", result.TotalUsage.InputTokens)
	}
}

func TestSessionFollowUpMultiple(t *testing.T) {
	callCount := 0
	prov := &customTestProvider{fn: func(ctx context.Context, req llm.Request) (*llm.Response, error) {
		callCount++
		return &llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, fmt.Sprintf("response %d", callCount)),
			FinishReason: llm.FinishStop,
			Usage:        llm.Usage{InputTokens: 10, OutputTokens: 5},
		}, nil
	}}

	client := llm.NewClient(llm.WithProvider(prov))
	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)

	// Queue multiple follow-ups.
	session.FollowUp("followup 1")
	session.FollowUp("followup 2")

	result := session.Run(context.Background(), "start")
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	// 1 initial + 2 follow-ups = 3 rounds.
	if callCount != 3 {
		t.Errorf("LLM call count = %d, want 3", callCount)
	}
	if result.Rounds != 3 {
		t.Errorf("rounds = %d, want 3", result.Rounds)
	}
	if result.FinalText != "response 3" {
		t.Errorf("final text = %q, want %q", result.FinalText, "response 3")
	}
}

func TestSessionFollowUpEvents(t *testing.T) {
	client := newTestClient(
		&llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "first"),
			FinishReason: llm.FinishStop,
		},
		&llm.Response{
			Message:      llm.TextMessage(llm.RoleAssistant, "second"),
			FinishReason: llm.FinishStop,
		},
	)

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}
	session := NewSession(cfg)
	session.FollowUp("follow-up")

	var mu sync.Mutex
	followupExitSeen := false
	session.OnEvent(func(e event.Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Type == event.TurnEnd {
			if exit, ok := e.Data["exit"]; ok && exit == "followup" {
				followupExitSeen = true
			}
		}
	})

	result := session.Run(context.Background(), "start")
	if result.Error != nil {
		t.Fatal(result.Error)
	}

	mu.Lock()
	defer mu.Unlock()
	if !followupExitSeen {
		t.Error("expected turn.end event with exit=followup")
	}
}
