package handler

import (
	"context"
	"fmt"
	"log/slog"

	"dfgo/internal/agent"
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
		}

		// Pass goal from pipeline context.
		if goal, ok := pctx.Get("goal"); ok {
			cfg.UserPrompt = fmt.Sprintf("Project goal: %s", goal)
		}

		return agent.NewSession(cfg)
	}
}
