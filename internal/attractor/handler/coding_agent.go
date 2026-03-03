package handler

import (
	"context"
	"fmt"
	"log/slog"

	"dfgo/internal/agent"
	"dfgo/internal/agent/event"
	"dfgo/internal/agent/execenv"
	"dfgo/internal/agent/profile"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
	"dfgo/internal/llm"
)

// AgentSessionFactory creates and configures an agent.Session for a pipeline node.
// This allows the pipeline to inject custom LLM clients, profiles, etc.
type AgentSessionFactory func(node *model.Node, pctx *runtime.Context, g *model.Graph) *agent.Session

// CodingAgentHandler executes a coding agent session as a pipeline stage.
// The agent receives the node's prompt, runs an autonomous tool loop, and
// returns the final text as a context update.
type CodingAgentHandler struct {
	factory AgentSessionFactory
}

// NewCodingAgentHandler creates a CodingAgentHandler with the given session factory.
func NewCodingAgentHandler(factory AgentSessionFactory) *CodingAgentHandler {
	return &CodingAgentHandler{factory: factory}
}

func (h *CodingAgentHandler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	prompt := node.StringAttr("prompt", "")
	if prompt == "" {
		return runtime.FailOutcome("no prompt attribute on coding_agent node", runtime.FailureDeterministic), nil
	}

	slog.Info("coding_agent: starting session", "node", node.ID)

	var session *agent.Session
	if h.factory != nil {
		session = h.factory(node, pctx, g)
	} else {
		// Default: create a stub session (no LLM client).
		slog.Warn("coding_agent: no session factory, returning stub", "node", node.ID)
		return runtime.Outcome{
			Status: runtime.StatusSuccess,
			Notes:  "stub: no agent session factory configured",
			ContextUpdates: map[string]string{
				node.ID + ".response": "(stub agent response)",
			},
		}, nil
	}

	// Subscribe to agent events for verbose logging.
	session.OnEvent(func(e event.Event) {
		switch e.Type {
		case event.TurnStart:
			slog.Debug("coding_agent: turn start", "node", node.ID, "round", e.Data["round"])
		case event.LLMResponse:
			slog.Debug("coding_agent: llm response",
				"node", node.ID,
				"finish_reason", e.Data["finish_reason"],
				"input_tokens", e.Data["input_tokens"],
				"output_tokens", e.Data["output_tokens"],
			)
		case event.LLMStreamStart:
			slog.Debug("coding_agent: stream started", "node", node.ID, "model", e.Data["model"])
		case event.LLMChunk:
			if text, _ := e.Data["text"].(string); text != "" {
				slog.Debug("coding_agent: chunk", "node", node.ID, "kind", e.Data["kind"], "text", text)
			}
		case event.LLMStreamEnd:
			slog.Debug("coding_agent: stream ended",
				"node", node.ID,
				"finish_reason", e.Data["finish_reason"],
				"input_tokens", e.Data["input_tokens"],
				"output_tokens", e.Data["output_tokens"],
			)
		case event.ToolStart:
			slog.Debug("coding_agent: tool call", "node", node.ID, "tool", e.Data["tool"])
		case event.ToolEnd:
			slog.Debug("coding_agent: tool done", "node", node.ID, "tool", e.Data["tool"], "is_error", e.Data["is_error"])
		case event.LLMError:
			slog.Warn("coding_agent: llm error", "node", node.ID, "error", e.Data["error"])
		case event.LoopDetected:
			slog.Warn("coding_agent: loop detected", "node", node.ID, "tool", e.Data["tool"])
		}
	})

	result := session.Run(ctx, prompt)

	if result.Error != nil {
		return runtime.FailOutcome(
			fmt.Sprintf("agent session error: %v", result.Error),
			runtime.FailureTransient,
		), nil
	}

	if result.Aborted {
		return runtime.FailOutcome("agent session aborted", runtime.FailureTransient), nil
	}

	slog.Info("coding_agent: session complete",
		"node", node.ID,
		"rounds", result.Rounds,
		"input_tokens", result.TotalUsage.InputTokens,
		"output_tokens", result.TotalUsage.OutputTokens,
	)

	return runtime.Outcome{
		Status: runtime.StatusSuccess,
		ContextUpdates: map[string]string{
			node.ID + ".response": result.FinalText,
		},
	}, nil
}

// DefaultAgentSessionFactory creates a factory using the given LLM client.
// It reads configuration from node attributes (model, max_rounds, etc.).
func DefaultAgentSessionFactory(client *llm.Client, env execenv.Environment) AgentSessionFactory {
	return func(node *model.Node, pctx *runtime.Context, g *model.Graph) *agent.Session {
		modelName := node.StringAttr("model", "claude-sonnet-4-20250514")
		maxRounds := node.IntAttr("max_rounds", 200)
		providerName := node.StringAttr("provider", "anthropic")

		var prof profile.Profile
		switch providerName {
		case "openai":
			prof = profile.OpenAI{}
		case "gemini":
			prof = profile.Gemini{}
		default:
			prof = profile.Anthropic{}
		}

		cfg := agent.Config{
			Client:    client,
			Profile:   prof,
			Env:       env,
			Model:     modelName,
			MaxRounds: maxRounds,
			Streaming: node.BoolAttr("stream", false),
		}

		// Pass goal from pipeline context.
		if goal, ok := pctx.Get("goal"); ok {
			cfg.UserPrompt = fmt.Sprintf("Project goal: %s", goal)
		}

		return agent.NewSession(cfg)
	}
}
