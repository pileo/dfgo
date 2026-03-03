package handler

import (
	"context"
	"strings"
	"time"

	"dfgo/internal/attractor/cond"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// ManagerLoopHandler implements the stack.manager_loop pattern (house shape).
// It manages iterative refinement by observing child status and waiting
// until a stop condition is met or max cycles are exhausted.
type ManagerLoopHandler struct {
	// ChildEngine is set by the engine to execute sub-pipelines.
	ChildEngine func(ctx context.Context, subgraphNodes []string) (runtime.Outcome, error)
}

func (h *ManagerLoopHandler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	// In stub mode, just succeed.
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

	// Read configuration attributes.
	pollInterval := node.DurationAttr("manager.poll_interval", 45*time.Second)
	maxCycles := node.IntAttr("manager.max_cycles", 1000)
	stopConditionStr := node.StringAttr("manager.stop_condition", "")
	actionsStr := node.StringAttr("manager.actions", "observe,wait")

	actions := parseActions(actionsStr)

	// Parse stop condition if provided.
	var stopExpr cond.Expr
	if stopConditionStr != "" {
		var err error
		stopExpr, err = cond.Parse(stopConditionStr)
		if err != nil {
			return runtime.FailOutcome("manager_loop: invalid stop_condition: "+err.Error(), runtime.FailureDeterministic), nil
		}
	}

	// Execute the child pipeline once to start.
	outcome, err := h.ChildEngine(ctx, children)
	if err != nil {
		return runtime.FailOutcome("manager_loop child error: "+err.Error(), runtime.FailureTransient), nil
	}

	// Supervision loop.
	for cycle := 1; cycle < maxCycles; cycle++ {
		// Check context cancellation.
		if ctx.Err() != nil {
			return runtime.FailOutcome("manager_loop canceled", runtime.FailureCanceled), nil
		}

		// Observe: check child status from context.
		if actions["observe"] {
			childStatus, _ := pctx.Get("stack.child.status")

			// Check if child has reached a terminal state.
			if childStatus == string(runtime.StatusSuccess) {
				return runtime.SuccessOutcome(), nil
			}
			if childStatus == string(runtime.StatusFail) {
				return runtime.FailOutcome("manager_loop: child failed", runtime.FailureTransient), nil
			}

			// Check custom stop condition.
			if stopConditionStr != "" {
				env := cond.Env{
					Outcome: string(outcome.Status),
					Context: pctx.Snapshot(),
				}
				if stopExpr.Eval(env) {
					return runtime.SuccessOutcome(), nil
				}
			}
		}

		// Wait: sleep for poll interval with cancellation support.
		if actions["wait"] {
			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
				return runtime.FailOutcome("manager_loop canceled", runtime.FailureCanceled), nil
			}
		}

		// Re-execute child pipeline for next cycle.
		outcome, err = h.ChildEngine(ctx, children)
		if err != nil {
			return runtime.FailOutcome("manager_loop child error: "+err.Error(), runtime.FailureTransient), nil
		}
	}

	// Max cycles exhausted.
	return runtime.FailOutcome("manager_loop: max cycles exceeded", runtime.FailureTransient), nil
}

// parseDuration parses a duration string, falling back to def on error.
// Delegates to model.ParseDuration for the actual parsing.
func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := model.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// parseActions splits a comma-separated actions string into a lookup set.
func parseActions(s string) map[string]bool {
	m := make(map[string]bool)
	for _, a := range strings.Split(s, ",") {
		a = strings.TrimSpace(a)
		if a != "" {
			m[a] = true
		}
	}
	return m
}
