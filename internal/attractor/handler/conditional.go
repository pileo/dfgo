package handler

import (
	"context"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// ConditionalHandler is a pass-through handler for diamond/conditional nodes.
// It simply succeeds; edge selection handles the routing logic.
type ConditionalHandler struct{}

func (h *ConditionalHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.SuccessOutcome(), nil
}
