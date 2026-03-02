package handler

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// JoinPolicy determines how parallel branches are joined.
type JoinPolicy string

const (
	JoinWaitAll      JoinPolicy = "wait_all"
	JoinKOfN         JoinPolicy = "k_of_n"
	JoinFirstSuccess JoinPolicy = "first_success"
	JoinQuorum       JoinPolicy = "quorum"
)

// ParallelHandler fans out to child nodes and joins results.
type ParallelHandler struct {
	// ChildExecutor is called to execute each child branch.
	// It's set by the engine at runtime.
	ChildExecutor func(ctx context.Context, nodeID string) (runtime.Outcome, error)
}

// NewParallelHandler creates a ParallelHandler.
func NewParallelHandler() *ParallelHandler {
	return &ParallelHandler{}
}

func (h *ParallelHandler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	children := g.Successors(node.ID)
	if len(children) == 0 {
		return runtime.SuccessOutcome(), nil
	}

	policy := JoinPolicy(node.StringAttr("join", string(JoinWaitAll)))

	if h.ChildExecutor == nil {
		// Stub mode: just succeed
		return runtime.Outcome{
			Status: runtime.StatusSuccess,
			Notes:  fmt.Sprintf("parallel stub: %d children, policy=%s", len(children), policy),
		}, nil
	}

	type result struct {
		nodeID  string
		outcome runtime.Outcome
		err     error
	}

	results := make([]result, len(children))
	var wg sync.WaitGroup

	for i, childID := range children {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			o, err := h.ChildExecutor(ctx, id)
			results[i] = result{nodeID: id, outcome: o, err: err}
		}(i, childID)
	}
	wg.Wait()

	outcomes := make(map[string]runtime.Outcome)
	errs := make(map[string]error)
	for _, r := range results {
		outcomes[r.nodeID] = r.outcome
		if r.err != nil {
			errs[r.nodeID] = r.err
		}
	}

	k := node.IntAttr("k", 0)
	return joinParallelResults(outcomes, errs, policy, k), nil
}

func joinParallelResults(outcomes map[string]runtime.Outcome, errors map[string]error, policy JoinPolicy, k int) runtime.Outcome {
	successCount := 0
	failCount := 0
	var failReasons []string

	for id, o := range outcomes {
		if errors[id] != nil {
			failCount++
			failReasons = append(failReasons, fmt.Sprintf("%s: error: %v", id, errors[id]))
			continue
		}
		if o.IsSuccess() {
			successCount++
		} else {
			failCount++
			if o.FailureReason != "" {
				failReasons = append(failReasons, fmt.Sprintf("%s: %s", id, o.FailureReason))
			}
		}
	}

	total := len(outcomes)

	switch policy {
	case JoinWaitAll:
		if failCount == 0 {
			return runtime.SuccessOutcome()
		}
		return runtime.FailOutcome(
			fmt.Sprintf("%d/%d branches failed: %s", failCount, total, strings.Join(failReasons, "; ")),
			runtime.FailureTransient,
		)

	case JoinFirstSuccess:
		if successCount > 0 {
			return runtime.SuccessOutcome()
		}
		return runtime.FailOutcome("no branch succeeded", runtime.FailureTransient)

	case JoinKOfN:
		if successCount >= k {
			return runtime.SuccessOutcome()
		}
		return runtime.FailOutcome(
			fmt.Sprintf("only %d/%d branches succeeded (need %d)", successCount, total, k),
			runtime.FailureTransient,
		)

	case JoinQuorum:
		if successCount > total/2 {
			return runtime.SuccessOutcome()
		}
		return runtime.FailOutcome(
			fmt.Sprintf("no quorum: %d/%d succeeded", successCount, total),
			runtime.FailureTransient,
		)
	}

	return runtime.SuccessOutcome()
}
