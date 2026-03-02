package handler

import (
	"context"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// ManagerLoopHandler implements the stack.manager_loop pattern (house shape).
// It manages iterative refinement by re-entering child nodes until a goal is met.
type ManagerLoopHandler struct {
	// ChildEngine is set by the engine to execute sub-pipelines.
	ChildEngine func(ctx context.Context, subgraphNodes []string) (runtime.Outcome, error)
}

func (h *ManagerLoopHandler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	// In stub mode, just succeed
	if h.ChildEngine == nil {
		return runtime.Outcome{
			Status: runtime.StatusSuccess,
			Notes:  "manager_loop stub: no child engine",
		}, nil
	}

	children := g.Successors(node.ID)
	if len(children) == 0 {
		return runtime.SuccessOutcome(), nil
	}

	outcome, err := h.ChildEngine(ctx, children)
	if err != nil {
		return runtime.FailOutcome("manager_loop child error: "+err.Error(), runtime.FailureTransient), nil
	}
	return outcome, nil
}
