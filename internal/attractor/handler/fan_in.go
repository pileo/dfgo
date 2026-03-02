package handler

import (
	"context"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// FanInHandler consolidates results from parallel branches.
// It's a synchronization point — succeeds when all predecessors have completed.
type FanInHandler struct{}

func (h *FanInHandler) Execute(_ context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, _ string) (runtime.Outcome, error) {
	// In the engine, fan-in waits for all predecessor branches.
	// The handler itself is a pass-through that signals consolidation.
	return runtime.SuccessOutcome(), nil
}
