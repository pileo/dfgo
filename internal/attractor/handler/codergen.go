package handler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// CodergenBackend is the interface for LLM backends used by the codergen handler.
type CodergenBackend interface {
	Generate(ctx context.Context, prompt string, opts map[string]string) (response string, err error)
}

// CodergenHandler executes an LLM-backed code generation stage.
type CodergenHandler struct {
	Backend CodergenBackend
}

// NewCodergenHandler creates a CodergenHandler with the given backend.
func NewCodergenHandler(backend CodergenBackend) *CodergenHandler {
	return &CodergenHandler{Backend: backend}
}

func (h *CodergenHandler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	prompt := node.StringAttr("prompt", "")
	if prompt == "" {
		return runtime.FailOutcome("no prompt attribute on codergen node", runtime.FailureDeterministic), nil
	}

	// Log the prompt
	if logsDir != "" {
		logPrompt(logsDir, node.ID, prompt)
	}

	if h.Backend == nil {
		slog.Warn("codergen: no backend configured, returning stub success", "node", node.ID)
		return runtime.Outcome{
			Status: runtime.StatusSuccess,
			Notes:  "stub: no backend configured",
			ContextUpdates: map[string]string{
				node.ID + ".response": "(stub response)",
			},
		}, nil
	}

	opts := map[string]string{
		"node_id": node.ID,
		"goal":    g.Attrs["goal"],
	}
	for k, v := range node.Attrs {
		if k != "prompt" && k != "shape" && k != "type" {
			opts[k] = v
		}
	}

	response, err := h.Backend.Generate(ctx, prompt, opts)
	if err != nil {
		return runtime.FailOutcome(fmt.Sprintf("codergen backend error: %v", err), runtime.FailureTransient), nil
	}

	if logsDir != "" {
		logResponse(logsDir, node.ID, response)
	}

	return runtime.Outcome{
		Status: runtime.StatusSuccess,
		ContextUpdates: map[string]string{
			node.ID + ".response": response,
		},
	}, nil
}

func logPrompt(logsDir, nodeID, prompt string) {
	dir := filepath.Join(logsDir, nodeID)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(prompt), 0o644)
}

func logResponse(logsDir, nodeID, response string) {
	dir := filepath.Join(logsDir, nodeID)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "response.txt"), []byte(response), 0o644)
}
