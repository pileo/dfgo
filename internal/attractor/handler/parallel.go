package handler

import (
	"context"
	"encoding/json"
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

// ErrorPolicy determines how parallel branches handle failures.
type ErrorPolicy string

const (
	ErrorPolicyContinue ErrorPolicy = "continue"  // default: run all branches regardless
	ErrorPolicyFailFast ErrorPolicy = "fail_fast"  // cancel remaining on first failure
	ErrorPolicyIgnore   ErrorPolicy = "ignore"     // filter out failures from join evaluation
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
	errPolicy := ErrorPolicy(node.StringAttr("error_policy", string(ErrorPolicyContinue)))
	maxParallel := node.IntAttr("max_parallel", 4)
	if maxParallel < 1 {
		maxParallel = 1
	}

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

	// Use a derived context for fail_fast cancellation.
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Semaphore to limit concurrency.
	sem := make(chan struct{}, maxParallel)

	var wg sync.WaitGroup
	for i, childID := range children {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-execCtx.Done():
				results[i] = result{nodeID: id, outcome: runtime.FailOutcome("canceled", runtime.FailureCanceled), err: execCtx.Err()}
				return
			}

			// Check for cancellation before executing.
			if execCtx.Err() != nil {
				results[i] = result{nodeID: id, outcome: runtime.FailOutcome("canceled", runtime.FailureCanceled), err: execCtx.Err()}
				return
			}

			o, err := h.ChildExecutor(execCtx, id)
			results[i] = result{nodeID: id, outcome: o, err: err}

			// For fail_fast: cancel remaining branches on first failure.
			if errPolicy == ErrorPolicyFailFast {
				if err != nil || !o.IsSuccess() {
					cancel()
				}
			}
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

	// Store results in context as JSON for fan-in consumption.
	storeParallelResults(pctx, outcomes)

	// For ignore policy, filter out failures before join evaluation.
	if errPolicy == ErrorPolicyIgnore {
		for id, o := range outcomes {
			if !o.IsSuccess() || errs[id] != nil {
				delete(outcomes, id)
				delete(errs, id)
			}
		}
		// If all were filtered out, that's a failure.
		if len(outcomes) == 0 {
			return runtime.FailOutcome("all branches failed (error_policy=ignore)", runtime.FailureTransient), nil
		}
	}

	k := node.IntAttr("k", 0)
	return joinParallelResults(outcomes, errs, policy, k), nil
}

// storeParallelResults writes outcomes to context as JSON for fan-in.
func storeParallelResults(pctx *runtime.Context, outcomes map[string]runtime.Outcome) {
	data, err := json.Marshal(outcomes)
	if err != nil {
		return
	}
	pctx.Set("parallel.results", string(data))
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
