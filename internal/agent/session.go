package agent

import (
	"context"
	"fmt"
	"sync"

	"dfgo/internal/agent/event"
	"dfgo/internal/agent/loop"
	"dfgo/internal/agent/message"
	"dfgo/internal/agent/profile"
	"dfgo/internal/agent/prompt"
	"dfgo/internal/agent/tool"
	"dfgo/internal/llm"
)

const (
	contextWindowThreshold = 0.80 // warn at 80% utilization
	consecutiveFailureMax  = 3    // circuit breaker threshold
)

// Session manages a single agent interaction: system prompt, message history,
// tool execution, and the core agentic loop.
type Session struct {
	cfg      Config
	registry *tool.Registry
	emitter  *event.Emitter
	detector *loop.Detector

	mu       sync.Mutex
	messages []message.Message
	steering []message.Message // pending steering messages
	followup []message.Message // queued follow-up messages
	usage    llm.Usage
	rounds   int
}

// NewSession creates a new agent session.
func NewSession(cfg Config) *Session {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 200
	}
	reg := profile.ConfigureRegistry(cfg.Profile)
	return &Session{
		cfg:      cfg,
		registry: reg,
		emitter:  event.NewEmitter(256),
		detector: loop.NewDetector(cfg.loopWindow()),
	}
}

// RegisterSubagentTools adds spawning/management tools backed by the given manager.
func (s *Session) RegisterSubagentTools(mgr *SubagentManager) {
	adapter := &subagentAdapter{mgr: mgr}
	s.registry.Register(tool.NewSpawnAgent(adapter))
	s.registry.Register(tool.NewSendInput(adapter))
	s.registry.Register(tool.NewWaitAgent(adapter))
	s.registry.Register(tool.NewCloseAgent(adapter))
}

// subagentAdapter bridges SubagentManager to tool.SubagentSpawner.
type subagentAdapter struct {
	mgr *SubagentManager
}

func (a *subagentAdapter) Spawn(ctx context.Context, id, input string) error {
	return a.mgr.Spawn(ctx, id, input)
}

func (a *subagentAdapter) SendInput(id, input string) error {
	return a.mgr.SendInput(id, input)
}

func (a *subagentAdapter) Wait(id string) (tool.SubagentResult, error) {
	r, err := a.mgr.Wait(id)
	if err != nil {
		return tool.SubagentResult{}, err
	}
	return tool.SubagentResult{
		FinalText: r.FinalText,
		Rounds:    r.Rounds,
		Aborted:   r.Aborted,
		Error:     r.Error,
	}, nil
}

func (a *subagentAdapter) Close(id string) error {
	return a.mgr.Close(id)
}

// OnEvent registers a callback for agent events.
func (s *Session) OnEvent(cb event.Callback) {
	s.emitter.On(cb)
}

// Steer injects a steering message into the next turn.
func (s *Session) Steer(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steering = append(s.steering, message.SteeringMessage(content))
}

// FollowUp queues a message to be processed after the main loop completes.
func (s *Session) FollowUp(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followup = append(s.followup, message.UserMessage(content))
}

// Run executes the agent loop with the given user input.
func (s *Session) Run(ctx context.Context, input string) Result {
	s.emitter.Emit(event.SessionStart, map[string]any{"model": s.cfg.Model})
	defer func() {
		s.emitter.Emit(event.SessionEnd, map[string]any{"rounds": s.rounds})
		s.emitter.Close()
	}()

	// Add user message.
	s.mu.Lock()
	s.messages = append(s.messages, message.UserMessage(input))
	s.mu.Unlock()

	consecutiveFailures := 0
	var lastText string

	for {
		// Check context cancellation.
		if ctx.Err() != nil {
			s.emitter.Emit(event.Abort, map[string]any{"reason": "context cancelled"})
			return s.result(lastText, true, nil)
		}

		// Check round limit.
		if s.rounds >= s.cfg.MaxRounds {
			s.emitter.Emit(event.Abort, map[string]any{"reason": "max rounds exceeded"})
			return s.result(lastText, true, nil)
		}

		s.rounds++
		s.emitter.Emit(event.TurnStart, map[string]any{"round": s.rounds})

		// Build LLM request.
		req := s.buildRequest()

		// Call LLM.
		s.emitter.Emit(event.LLMRequest, map[string]any{
			"model":    req.Model,
			"messages": len(req.Messages),
		})

		var resp *llm.Response
		var err error
		if s.cfg.Streaming {
			resp, err = s.streamTurn(ctx, req)
		} else {
			resp, err = s.cfg.Client.Complete(ctx, req)
		}
		if err != nil {
			s.emitter.Emit(event.LLMError, map[string]any{"error": err.Error()})
			consecutiveFailures++

			if consecutiveFailures >= consecutiveFailureMax {
				return s.result(lastText, false, fmt.Errorf("circuit breaker: %d consecutive failures: %w", consecutiveFailures, err))
			}

			if llm.IsRetryable(err) {
				continue
			}
			return s.result(lastText, false, err)
		}
		consecutiveFailures = 0

		// Accumulate usage.
		s.mu.Lock()
		s.usage.InputTokens += resp.Usage.InputTokens
		s.usage.OutputTokens += resp.Usage.OutputTokens
		s.usage.TotalTokens += resp.Usage.TotalTokens
		s.usage.ReasoningTokens += resp.Usage.ReasoningTokens
		s.usage.CacheReadTokens += resp.Usage.CacheReadTokens
		s.usage.CacheWriteTokens += resp.Usage.CacheWriteTokens
		s.mu.Unlock()

		s.emitter.Emit(event.LLMResponse, map[string]any{
			"model":         req.Model,
			"finish_reason": string(resp.FinishReason),
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
			"text":          truncateResult(resp.Text(), 4000),
		})

		// Translate response to agent message.
		assistantMsg := s.responseToMessage(resp)
		s.mu.Lock()
		s.messages = append(s.messages, assistantMsg)
		s.mu.Unlock()

		if text := resp.Text(); text != "" {
			lastText = text
		}

		// Check context window utilization.
		s.checkContextWindow(resp)

		// If no tool calls, check for follow-up messages or exit.
		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			s.mu.Lock()
			hasFollowup := len(s.followup) > 0
			var nextMsg message.Message
			if hasFollowup {
				nextMsg = s.followup[0]
				s.followup = s.followup[1:]
			}
			s.mu.Unlock()

			if hasFollowup {
				s.emitter.Emit(event.TurnEnd, map[string]any{"round": s.rounds, "exit": "followup"})
				s.mu.Lock()
				s.messages = append(s.messages, nextMsg)
				s.mu.Unlock()
				continue
			}

			s.emitter.Emit(event.TurnEnd, map[string]any{"round": s.rounds, "exit": "natural"})
			return s.result(lastText, false, nil)
		}

		// Execute tool calls.
		for _, tc := range toolCalls {
			s.emitter.Emit(event.ToolStart, map[string]any{
				"tool":    tc.Name,
				"call_id": tc.ID,
			})

			// Loop detection.
			if s.cfg.loopEnabled() && s.detector.Record(tc.Name, string(tc.Arguments)) {
				s.emitter.Emit(event.LoopDetected, map[string]any{"tool": tc.Name})
				s.Steer("WARNING: Repetitive tool call pattern detected. Please try a different approach.")
			}

			result, execErr := s.registry.Execute(ctx, s.cfg.Env, tc.Name, tc.Arguments)
			if execErr != nil {
				s.emitter.Emit(event.ToolError, map[string]any{
					"tool":  tc.Name,
					"error": execErr.Error(),
				})
				result = tool.ErrorResult(fmt.Sprintf("tool execution error: %v", execErr))
			}

			s.emitter.Emit(event.ToolEnd, map[string]any{
				"tool":     tc.Name,
				"call_id":  tc.ID,
				"args":     string(tc.Arguments),
				"result":   truncateResult(result.Content, 4000),
				"is_error": result.IsError,
			})

			// Add tool result to conversation.
			toolMsg := message.Message{
				Role: message.RoleTool,
				ToolResult: &message.ToolResult{
					ToolCallID: tc.ID,
					Content:    result.Content,
					FullOutput: result.FullOutput,
					IsError:    result.IsError,
				},
			}
			s.mu.Lock()
			s.messages = append(s.messages, toolMsg)
			s.mu.Unlock()
		}

		// Inject any pending steering messages.
		s.mu.Lock()
		if len(s.steering) > 0 {
			for _, sm := range s.steering {
				s.emitter.Emit(event.Steering, map[string]any{"content": sm.Content})
			}
			s.messages = append(s.messages, s.steering...)
			s.steering = s.steering[:0]
		}
		s.mu.Unlock()

		s.emitter.Emit(event.TurnEnd, map[string]any{"round": s.rounds})
	}
}

// streamTurn performs a single LLM turn using streaming, emitting real-time
// events and returning the accumulated response.
func (s *Session) streamTurn(ctx context.Context, req llm.Request) (*llm.Response, error) {
	stream, err := s.cfg.Client.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	for stream.Next() {
		evt := stream.Event()
		switch evt.Type {
		case llm.EventResponseMeta:
			s.emitter.Emit(event.LLMStreamStart, map[string]any{
				"response_id": evt.ResponseID,
				"model":       evt.Model,
			})
		case llm.EventContentDelta:
			s.emitter.Emit(event.LLMChunk, map[string]any{
				"kind":  string(evt.ContentKind),
				"text":  evt.Text,
				"index": evt.Index,
			})
		case llm.EventError:
			return nil, evt.Err
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	resp := stream.Response()
	if resp == nil {
		return nil, &llm.SDKError{Message: "stream completed without response"}
	}

	s.emitter.Emit(event.LLMStreamEnd, map[string]any{
		"finish_reason": string(resp.FinishReason),
		"input_tokens":  resp.Usage.InputTokens,
		"output_tokens": resp.Usage.OutputTokens,
	})

	return resp, nil
}

// buildRequest translates the agent state into an LLM SDK Request.
func (s *Session) buildRequest() llm.Request {
	// Build system prompt.
	pb := prompt.NewBuilder(s.cfg.Profile, s.registry, s.cfg.Env.WorkingDir()).
		WithModel(s.cfg.Model).
		WithPlatformInfo()
	projectDoc := s.cfg.ProjectDoc
	if projectDoc == "" {
		projectDoc = prompt.DiscoverProjectDocs(s.cfg.Env.WorkingDir(), s.cfg.Profile.Name())
	}
	if projectDoc != "" {
		pb.WithProjectDoc(projectDoc)
	}
	if s.cfg.UserPrompt != "" {
		pb.WithUserPrompt(s.cfg.UserPrompt)
	}
	systemPrompt := pb.Build()

	// Translate agent messages to LLM messages.
	s.mu.Lock()
	msgs := make([]message.Message, len(s.messages))
	copy(msgs, s.messages)
	s.mu.Unlock()

	llmMsgs := make([]llm.Message, 0, len(msgs)+1)
	llmMsgs = append(llmMsgs, llm.TextMessage(llm.RoleSystem, systemPrompt))

	for _, m := range msgs {
		llmMsgs = append(llmMsgs, agentToLLMMessage(m))
	}

	req := llm.Request{
		Model:           s.cfg.Model,
		Messages:        llmMsgs,
		Tools:           s.registry.ToolDefs(),
		Temperature:     s.cfg.Temperature,
		MaxTokens:       s.cfg.MaxTokens,
		ReasoningEffort: s.cfg.ReasoningEffort,
	}
	return req
}

// agentToLLMMessage converts an agent message to an LLM SDK message.
func agentToLLMMessage(m message.Message) llm.Message {
	switch m.Role {
	case message.RoleAssistant:
		msg := llm.Message{Role: llm.RoleAssistant}
		if m.Content != "" {
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentText,
				Text: m.Content,
			})
		}
		for _, tc := range m.ToolCalls {
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentToolCall,
				ToolCall: &llm.ToolCallData{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Args,
				},
			})
		}
		return msg

	case message.RoleTool:
		if m.ToolResult != nil {
			return llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: m.ToolResult.ToolCallID,
				Content: []llm.ContentPart{{
					Kind: llm.ContentToolResult,
					ToolResult: &llm.ToolResultData{
						ToolCallID: m.ToolResult.ToolCallID,
						Content:    m.ToolResult.Content,
						IsError:    m.ToolResult.IsError,
					},
				}},
			}
		}
		return llm.TextMessage(llm.RoleUser, m.Content)

	case message.RoleSteering:
		// Steering messages appear as user messages to the model.
		return llm.TextMessage(llm.RoleUser, "[SYSTEM] "+m.Content)

	default:
		return llm.TextMessage(llm.RoleUser, m.Content)
	}
}

// responseToMessage converts an LLM response to an agent message.
func (s *Session) responseToMessage(resp *llm.Response) message.Message {
	msg := message.Message{
		Role:    message.RoleAssistant,
		Content: resp.Text(),
	}
	for _, tc := range resp.ToolCalls() {
		msg.ToolCalls = append(msg.ToolCalls, message.ToolCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: tc.Arguments,
		})
	}
	return msg
}

// checkContextWindow estimates context utilization and emits a warning.
func (s *Session) checkContextWindow(resp *llm.Response) {
	windowSize := s.cfg.Profile.ContextWindowSize()
	if windowSize <= 0 {
		return
	}
	utilization := float64(resp.Usage.InputTokens) / float64(windowSize)
	if utilization >= contextWindowThreshold {
		s.emitter.Emit(event.ContextTruncate, map[string]any{
			"utilization":   utilization,
			"input_tokens":  resp.Usage.InputTokens,
			"window_size":   windowSize,
		})
		s.Steer(fmt.Sprintf("WARNING: Context window is %.0f%% full. Consider summarizing your work and completing the task soon.", utilization*100))
	}
}

func (s *Session) result(text string, aborted bool, err error) Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]message.Message, len(s.messages))
	copy(msgs, s.messages)
	return Result{
		Messages:   msgs,
		FinalText:  text,
		TotalUsage: s.usage,
		Rounds:     s.rounds,
		Aborted:    aborted,
		Error:      err,
	}
}

// Messages returns a snapshot of the conversation history.
func (s *Session) Messages() []message.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]message.Message, len(s.messages))
	copy(msgs, s.messages)
	return msgs
}

// Usage returns cumulative token usage.
func (s *Session) Usage() llm.Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

// ToolRegistry returns the session's tool registry for external registration.
func (s *Session) ToolRegistry() *tool.Registry {
	return s.registry
}

// truncateResult caps a string to maxLen, appending a marker if truncated.
func truncateResult(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…[truncated]"
}

