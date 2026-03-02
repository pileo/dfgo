package handler

import (
	"context"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// ExitHandler is the handler for exit nodes (Msquare shape). It's a no-op.
type ExitHandler struct{}

func (h *ExitHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.SuccessOutcome(), nil
}
