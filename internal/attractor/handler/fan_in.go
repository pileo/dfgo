package handler

import (
	"context"
	"encoding/json"
	"sort"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// FanInHandler consolidates results from parallel branches.
// It reads parallel.results from context, ranks candidates by status priority,
// and writes the best result to context for downstream consumption.
type FanInHandler struct{}

// statusPriority returns a sort priority for each status (lower = better).
func statusPriority(s runtime.StageStatus) int {
	switch s {
	case runtime.StatusSuccess:
		return 0
	case runtime.StatusPartialSuccess:
		return 1
	case runtime.StatusRetry:
		return 2
	case runtime.StatusFail:
		return 3
	default:
		return 4
	}
}

type rankedResult struct {
	ID      string
	Outcome runtime.Outcome
}

func (h *FanInHandler) Execute(_ context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, _ string) (runtime.Outcome, error) {
	raw, ok := pctx.Get("parallel.results")
	if !ok || raw == "" {
		// No parallel results — pass-through synchronization point.
		return runtime.SuccessOutcome(), nil
	}

	var results map[string]runtime.Outcome
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return runtime.FailOutcome("fan_in: failed to parse parallel.results: "+err.Error(), runtime.FailureDeterministic), nil
	}

	if len(results) == 0 {
		return runtime.SuccessOutcome(), nil
	}

	// Build ranked list.
	ranked := make([]rankedResult, 0, len(results))
	for id, o := range results {
		ranked = append(ranked, rankedResult{ID: id, Outcome: o})
	}

	// Sort by status priority (ascending), then by ID (ascending) for stability.
	sort.Slice(ranked, func(i, j int) bool {
		pi := statusPriority(ranked[i].Outcome.Status)
		pj := statusPriority(ranked[j].Outcome.Status)
		if pi != pj {
			return pi < pj
		}
		return ranked[i].ID < ranked[j].ID
	})

	best := ranked[0]

	// Write best result to context for downstream nodes.
	pctx.Set("parallel.fan_in.best_id", best.ID)
	bestJSON, _ := json.Marshal(best.Outcome)
	pctx.Set("parallel.fan_in.best_outcome", string(bestJSON))

	// If all candidates failed, return FAIL.
	allFailed := true
	for _, r := range ranked {
		if r.Outcome.IsSuccess() {
			allFailed = false
			break
		}
	}
	if allFailed {
		return runtime.FailOutcome("fan_in: all candidates failed", runtime.FailureTransient), nil
	}

	return runtime.SuccessOutcome(), nil
}
