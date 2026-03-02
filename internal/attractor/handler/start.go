package handler

import (
	"context"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// StartHandler is the handler for the start node (Mdiamond shape). It's a no-op.
type StartHandler struct{}

func (h *StartHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.SuccessOutcome(), nil
}
