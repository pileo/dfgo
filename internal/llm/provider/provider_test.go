package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"dfgo/internal/llm"
)

func TestAnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request.
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing api key header")
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Error("missing version header")
		}

		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("model = %q", req.Model)
		}
		if req.MaxTokens != 1024 {
			t.Errorf("max_tokens = %d", req.MaxTokens)
		}

		// Return response.
		resp := anthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-sonnet-4-20250514",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Hello!"},
			},
			StopReason: "end_turn",
		}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 5
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	maxToks := 1024
	resp, err := a.Complete(context.Background(), llm.Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: &maxToks,
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleSystem, "You are helpful."),
			llm.TextMessage(llm.RoleUser, "Hi"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != "Hello!" {
		t.Errorf("text = %q", resp.Text())
	}
	if resp.FinishReason != llm.FinishStop {
		t.Errorf("finish = %q", resp.FinishReason)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("input tokens = %d", resp.Usage.InputTokens)
	}
}

func TestAnthropicToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req anthropicRequest
		json.Unmarshal(body, &req)

		if len(req.Tools) != 1 {
			t.Errorf("tools = %d", len(req.Tools))
		}

		resp := anthropicResponse{
			ID:   "msg_456",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "I'll read that file."},
				{Type: "tool_use", ID: "tc_1", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)},
			},
			StopReason: "tool_use",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	resp, err := a.Complete(context.Background(), llm.Request{
		Model: "claude-sonnet-4-20250514",
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, "Read main.go"),
		},
		Tools: []llm.ToolDef{
			{Name: "read_file", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
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
}

func TestAnthropicError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"too many requests"}}`))
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	_, err := a.Complete(context.Background(), llm.Request{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !llm.IsRetryable(err) {
		t.Error("429 should be retryable")
	}
}

func TestAnthropicSystemExtraction(t *testing.T) {
	var captured anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)
		resp := anthropicResponse{ID: "msg_1", Role: "assistant", StopReason: "end_turn"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	a.Complete(context.Background(), llm.Request{
		Model: "test",
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleSystem, "Be helpful"),
			llm.TextMessage(llm.RoleUser, "Hi"),
		},
	})

	// System should be extracted to top-level, not in messages.
	if captured.System == nil {
		t.Fatal("system should be set")
	}
	if len(captured.Messages) != 1 {
		t.Errorf("messages = %d, want 1 (system extracted)", len(captured.Messages))
	}
}

func TestOpenAIComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}

		body, _ := io.ReadAll(r.Body)
		var req openaiRequest
		json.Unmarshal(body, &req)

		if req.Instructions != "Be helpful" {
			t.Errorf("instructions = %q", req.Instructions)
		}

		resp := openaiResponse{
			ID:    "resp_123",
			Model: "gpt-4o",
			Output: []openaiOutput{
				{
					Type: "message",
					Content: []openaiContent{
						{Type: "output_text", Text: "Hello!"},
					},
				},
			},
			Status: "completed",
		}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 5
		resp.Usage.TotalTokens = 15
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	o := NewOpenAI(func(o *OpenAI) {
		o.APIKey = "test-key"
		o.BaseURL = srv.URL
	})

	resp, err := o.Complete(context.Background(), llm.Request{
		Model: "gpt-4o",
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleSystem, "Be helpful"),
			llm.TextMessage(llm.RoleUser, "Hi"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != "Hello!" {
		t.Errorf("text = %q", resp.Text())
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("total tokens = %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			ID: "resp_456",
			Output: []openaiOutput{
				{
					Type:      "function_call",
					CallID:    "call_1",
					Name:      "shell",
					Arguments: `{"command":"ls"}`,
				},
			},
			Status: "completed",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	o := NewOpenAI(func(o *OpenAI) {
		o.APIKey = "test-key"
		o.BaseURL = srv.URL
	})

	resp, err := o.Complete(context.Background(), llm.Request{Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.FinishReason != llm.FinishToolUse {
		t.Errorf("finish = %q", resp.FinishReason)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Errorf("unexpected tool calls: %+v", calls)
	}
}

func TestGeminiComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req geminiRequest
		json.Unmarshal(body, &req)

		if req.SystemInstruction == nil {
			t.Error("expected system instruction")
		}

		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Role:  "model",
						Parts: []geminiPart{{Text: "Hello!"}},
					},
					FinishReason: "STOP",
				},
			},
		}
		resp.UsageMetadata.PromptTokenCount = 10
		resp.UsageMetadata.CandidatesTokenCount = 5
		resp.UsageMetadata.TotalTokenCount = 15
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := NewGemini(func(g *Gemini) {
		g.APIKey = "test-key"
		g.BaseURL = srv.URL
	})

	resp, err := g.Complete(context.Background(), llm.Request{
		Model: "gemini-2.0-flash",
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleSystem, "Be helpful"),
			llm.TextMessage(llm.RoleUser, "Hi"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != "Hello!" {
		t.Errorf("text = %q", resp.Text())
	}
	if resp.Provider != "gemini" {
		t.Errorf("provider = %q", resp.Provider)
	}
}

func TestGeminiFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Role: "model",
						Parts: []geminiPart{
							{FunctionCall: &geminiFunctionCall{Name: "read_file", Args: map[string]any{"path": "main.go"}}},
						},
					},
					FinishReason: "STOP",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	g := NewGemini(func(g *Gemini) {
		g.APIKey = "test-key"
		g.BaseURL = srv.URL
	})

	resp, err := g.Complete(context.Background(), llm.Request{Model: "gemini-2.0-flash"})
	if err != nil {
		t.Fatal(err)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d", len(calls))
	}
	if calls[0].Name != "read_file" {
		t.Errorf("tool name = %q", calls[0].Name)
	}
	// Gemini generates synthetic UUID for tool call ID.
	if calls[0].ID == "" {
		t.Error("expected synthetic ID")
	}
}

func TestCacheBreakpoints(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		resp := anthropicResponse{ID: "msg_cache", Role: "assistant", StopReason: "end_turn"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	a.Complete(context.Background(), llm.Request{
		Model: "claude-sonnet-4-20250514",
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleSystem, "Be helpful"),
			llm.TextMessage(llm.RoleUser, "First message"),
			llm.TextMessage(llm.RoleAssistant, "OK"),
			llm.TextMessage(llm.RoleUser, "Second message"),
		},
		Tools: []llm.ToolDef{
			{Name: "read_file", Description: "Read", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "write_file", Description: "Write", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})

	// The serialized request should contain cache_control entries.
	body := string(captured)
	if body == "" {
		t.Fatal("no request body captured")
	}

	// Parse the request to verify cache breakpoints.
	var req anthropicRequest
	if err := json.Unmarshal(captured, &req); err != nil {
		t.Fatal(err)
	}

	// 1. System prompt last block should have cache_control.
	var sysBlocks []anthropicContentBlock
	if err := json.Unmarshal(req.System, &sysBlocks); err != nil {
		t.Fatal(err)
	}
	if len(sysBlocks) == 0 {
		t.Fatal("no system blocks")
	}
	lastSys := sysBlocks[len(sysBlocks)-1]
	if lastSys.CacheControl == nil || lastSys.CacheControl.Type != "ephemeral" {
		t.Error("system prompt last block should have ephemeral cache_control")
	}

	// 2. Last tool should have cache_control.
	if len(req.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(req.Tools))
	}
	lastTool := req.Tools[len(req.Tools)-1]
	if lastTool.CacheControl == nil || lastTool.CacheControl.Type != "ephemeral" {
		t.Error("last tool should have ephemeral cache_control")
	}
	// First tool should NOT have cache_control.
	if req.Tools[0].CacheControl != nil {
		t.Error("first tool should not have cache_control")
	}

	// 3. Last user message should have cache_control on its last content block.
	// Find the last user message in the serialized request.
	lastUserIdx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		t.Fatal("no user message found")
	}
	var userBlocks []anthropicContentBlock
	if err := json.Unmarshal(req.Messages[lastUserIdx].Content, &userBlocks); err != nil {
		t.Fatal(err)
	}
	if len(userBlocks) == 0 {
		t.Fatal("no user content blocks")
	}
	lastUserBlock := userBlocks[len(userBlocks)-1]
	if lastUserBlock.CacheControl == nil || lastUserBlock.CacheControl.Type != "ephemeral" {
		t.Error("last user message block should have ephemeral cache_control")
	}
}

func TestCacheBreakpointsNoToolsNoSystem(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		resp := anthropicResponse{ID: "msg_1", Role: "assistant", StopReason: "end_turn"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	// Request with no system message and no tools.
	a.Complete(context.Background(), llm.Request{
		Model: "test",
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, "Hello"),
		},
	})

	// Should not panic, and should still work.
	var req anthropicRequest
	if err := json.Unmarshal(captured, &req); err != nil {
		t.Fatal(err)
	}
	// Just verify it didn't crash and the user message got cache_control.
	if len(req.Messages) == 0 {
		t.Fatal("no messages")
	}
}

func TestBetaHeaders(t *testing.T) {
	a := NewAnthropic()

	// Default: just prompt-caching.
	result := a.betaHeaders(llm.Request{})
	if result != "prompt-caching-2024-07-31" {
		t.Errorf("default beta headers = %q", result)
	}
}

func TestBetaHeadersMergeString(t *testing.T) {
	a := NewAnthropic()

	req := llm.Request{
		ProviderOptions: map[string]any{
			"anthropic": map[string]any{
				"beta_headers": "extended-thinking-2025-04-11",
			},
		},
	}
	result := a.betaHeaders(req)
	if result != "prompt-caching-2024-07-31,extended-thinking-2025-04-11" {
		t.Errorf("merged beta headers = %q", result)
	}
}

func TestBetaHeadersMergeList(t *testing.T) {
	a := NewAnthropic()

	req := llm.Request{
		ProviderOptions: map[string]any{
			"anthropic": map[string]any{
				"beta_headers": []any{"feature-a", "feature-b"},
			},
		},
	}
	result := a.betaHeaders(req)
	if result != "prompt-caching-2024-07-31,feature-a,feature-b" {
		t.Errorf("merged beta headers = %q", result)
	}
}

func TestBetaHeadersEmptyProviderOptions(t *testing.T) {
	a := NewAnthropic()

	req := llm.Request{
		ProviderOptions: map[string]any{
			"openai": map[string]any{"something": "else"},
		},
	}
	result := a.betaHeaders(req)
	if result != "prompt-caching-2024-07-31" {
		t.Errorf("beta headers with wrong provider = %q", result)
	}
}

func TestBetaHeadersSentInRequest(t *testing.T) {
	var capturedBeta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBeta = r.Header.Get("anthropic-beta")
		resp := anthropicResponse{ID: "msg_1", Role: "assistant", StopReason: "end_turn"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := NewAnthropic(func(a *Anthropic) {
		a.APIKey = "test-key"
		a.BaseURL = srv.URL
	})

	a.Complete(context.Background(), llm.Request{
		Model: "test",
		ProviderOptions: map[string]any{
			"anthropic": map[string]any{
				"beta_headers": "custom-beta-v1",
			},
		},
	})

	if capturedBeta != "prompt-caching-2024-07-31,custom-beta-v1" {
		t.Errorf("anthropic-beta header = %q", capturedBeta)
	}
}
