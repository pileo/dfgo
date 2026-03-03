package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
	"dfgo/internal/llm"
)

// mockLLMProvider implements llm.ProviderAdapter for handler tests.
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) Name() string { return "mock" }
func (m *mockLLMProvider) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	return &llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, m.response),
		FinishReason: llm.FinishStop,
	}, nil
}

func newMockClient(prov llm.ProviderAdapter) *llm.Client {
	return llm.NewClient(llm.WithProvider(prov))
}

func TestStartHandler(t *testing.T) {
	h := &StartHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "start"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("start handler should succeed")
	}
}

func TestExitHandler(t *testing.T) {
	h := &ExitHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "exit"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("exit handler should succeed")
	}
}

func TestConditionalHandler(t *testing.T) {
	h := &ConditionalHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "check"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("conditional handler should succeed")
	}
}

func TestCodergenHandlerStub(t *testing.T) {
	h := NewCodergenHandler(nil)
	node := &model.Node{ID: "gen", Attrs: map[string]string{"prompt": "write code"}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s", o.Status)
	}
	if o.ContextUpdates["gen.response"] == "" {
		t.Fatal("expected stub response in context updates")
	}
}

func TestCodergenHandlerNoPrompt(t *testing.T) {
	h := NewCodergenHandler(nil)
	node := &model.Node{ID: "gen", Attrs: map[string]string{}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected fail for missing prompt")
	}
}

type mockBackend struct {
	response string
	err      error
}

func (m *mockBackend) Generate(_ context.Context, prompt string, opts map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestCodergenHandlerWithBackend(t *testing.T) {
	h := NewCodergenHandler(&mockBackend{response: "generated code"})
	node := &model.Node{ID: "gen", Attrs: map[string]string{"prompt": "write code"}}
	dir := t.TempDir()
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", o.Status, o.FailureReason)
	}
	if o.ContextUpdates["gen.response"] != "generated code" {
		t.Fatal("expected response in context updates")
	}
}

func TestWaitHumanHandler(t *testing.T) {
	iv := interviewer.NewQueue("yes")
	h := NewWaitHumanHandler(iv)

	g := model.NewGraph("test")
	node := &model.Node{ID: "approval", Attrs: map[string]string{"prompt": "Approve this?"}}
	g.AddNode(node)

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatal("expected success")
	}
	if o.PreferredLabel != "yes" {
		t.Fatalf("expected preferred label 'yes', got %q", o.PreferredLabel)
	}
}

func TestWaitHumanHandlerMultipleChoice(t *testing.T) {
	iv := interviewer.NewQueue("approve")
	h := NewWaitHumanHandler(iv)

	g := model.NewGraph("test")
	node := &model.Node{ID: "review", Attrs: map[string]string{"prompt": "Review result"}}
	g.AddNode(node)
	g.AddEdge(&model.Edge{From: "review", To: "next", Attrs: map[string]string{"label": "approve"}, Order: 0})
	g.AddEdge(&model.Edge{From: "review", To: "redo", Attrs: map[string]string{"label": "reject"}, Order: 1})

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.PreferredLabel != "approve" {
		t.Fatalf("expected 'approve', got %q", o.PreferredLabel)
	}
}

func TestFanInHandlerNoResults(t *testing.T) {
	h := &FanInHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "join"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("fan-in should succeed with no parallel.results")
	}
}

func TestFanInHandlerRanking(t *testing.T) {
	pctx := runtime.NewContext()
	results := map[string]runtime.Outcome{
		"branch_c": {Status: runtime.StatusFail, FailureReason: "timeout"},
		"branch_a": {Status: runtime.StatusSuccess},
		"branch_b": {Status: runtime.StatusPartialSuccess},
	}
	data, _ := json.Marshal(results)
	pctx.Set("parallel.results", string(data))

	h := &FanInHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "join"}, pctx, model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s", o.Status)
	}

	bestID, _ := pctx.Get("parallel.fan_in.best_id")
	if bestID != "branch_a" {
		t.Fatalf("expected best_id=branch_a, got %q", bestID)
	}

	bestJSON, _ := pctx.Get("parallel.fan_in.best_outcome")
	var best runtime.Outcome
	if err := json.Unmarshal([]byte(bestJSON), &best); err != nil {
		t.Fatalf("failed to parse best_outcome: %v", err)
	}
	if best.Status != runtime.StatusSuccess {
		t.Fatalf("expected best outcome SUCCESS, got %s", best.Status)
	}
}

func TestFanInHandlerAllFailed(t *testing.T) {
	pctx := runtime.NewContext()
	results := map[string]runtime.Outcome{
		"a": {Status: runtime.StatusFail, FailureReason: "error1"},
		"b": {Status: runtime.StatusFail, FailureReason: "error2"},
	}
	data, _ := json.Marshal(results)
	pctx.Set("parallel.results", string(data))

	h := &FanInHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "join"}, pctx, model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatalf("expected FAIL when all candidates failed, got %s", o.Status)
	}

	// Should still pick the best (alphabetically first) even when all failed.
	bestID, _ := pctx.Get("parallel.fan_in.best_id")
	if bestID != "a" {
		t.Fatalf("expected best_id=a (alphabetical tiebreak), got %q", bestID)
	}
}

func TestFanInHandlerTiebreakByID(t *testing.T) {
	pctx := runtime.NewContext()
	results := map[string]runtime.Outcome{
		"z_branch": {Status: runtime.StatusSuccess},
		"a_branch": {Status: runtime.StatusSuccess},
		"m_branch": {Status: runtime.StatusSuccess},
	}
	data, _ := json.Marshal(results)
	pctx.Set("parallel.results", string(data))

	h := &FanInHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "join"}, pctx, model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("expected success")
	}

	bestID, _ := pctx.Get("parallel.fan_in.best_id")
	if bestID != "a_branch" {
		t.Fatalf("expected a_branch (alphabetical tiebreak), got %q", bestID)
	}
}

func TestFanInHandlerInvalidJSON(t *testing.T) {
	pctx := runtime.NewContext()
	pctx.Set("parallel.results", "not-json")

	h := &FanInHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "join"}, pctx, model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected FAIL for invalid JSON")
	}
}

func TestParallelHandlerStub(t *testing.T) {
	h := NewParallelHandler()
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{"join": "wait_all"}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "a", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "b", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "par", To: "a", Order: 0})
	g.AddEdge(&model.Edge{From: "par", To: "b", Order: 1})

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("parallel stub should succeed")
	}
}

func TestJoinParallelResultsWaitAll(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.SuccessOutcome(),
		"b": runtime.SuccessOutcome(),
	}
	o := joinParallelResults(outcomes, nil, JoinWaitAll, 0)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("wait_all should succeed when all succeed")
	}

	outcomes["b"] = runtime.FailOutcome("bad", runtime.FailureTransient)
	o = joinParallelResults(outcomes, nil, JoinWaitAll, 0)
	if o.Status != runtime.StatusFail {
		t.Fatal("wait_all should fail when any fails")
	}
}

func TestJoinParallelResultsFirstSuccess(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.FailOutcome("bad", runtime.FailureTransient),
		"b": runtime.SuccessOutcome(),
	}
	o := joinParallelResults(outcomes, nil, JoinFirstSuccess, 0)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("first_success should succeed when at least one succeeds")
	}
}

func TestJoinParallelResultsKOfN(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.SuccessOutcome(),
		"b": runtime.FailOutcome("bad", runtime.FailureTransient),
		"c": runtime.SuccessOutcome(),
	}
	o := joinParallelResults(outcomes, nil, JoinKOfN, 2)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("k_of_n(2) should succeed with 2/3")
	}

	o = joinParallelResults(outcomes, nil, JoinKOfN, 3)
	if o.Status != runtime.StatusFail {
		t.Fatal("k_of_n(3) should fail with 2/3")
	}
}

func TestJoinParallelResultsQuorum(t *testing.T) {
	outcomes := map[string]runtime.Outcome{
		"a": runtime.SuccessOutcome(),
		"b": runtime.SuccessOutcome(),
		"c": runtime.FailOutcome("bad", runtime.FailureTransient),
	}
	o := joinParallelResults(outcomes, nil, JoinQuorum, 0)
	if o.Status != runtime.StatusSuccess {
		t.Fatal("quorum should succeed with 2/3")
	}
}

func TestManagerLoopHandlerStub(t *testing.T) {
	h := &ManagerLoopHandler{}
	o, err := h.Execute(context.Background(), &model.Node{ID: "loop"}, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatal("manager_loop stub should succeed")
	}
}

// --- Parallel handler: error_policy, max_parallel, results storage ---

func TestParallelHandlerStoresResults(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "a", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "b", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "par", To: "a", Order: 0})
	g.AddEdge(&model.Edge{From: "par", To: "b", Order: 1})

	h := &ParallelHandler{
		ChildExecutor: func(_ context.Context, nodeID string) (runtime.Outcome, error) {
			return runtime.SuccessOutcome(), nil
		},
	}
	pctx := runtime.NewContext()
	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s: %v", o.Status, err)
	}

	raw, ok := pctx.Get("parallel.results")
	if !ok {
		t.Fatal("expected parallel.results in context")
	}
	var results map[string]runtime.Outcome
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		t.Fatalf("failed to parse parallel.results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, id := range []string{"a", "b"} {
		if results[id].Status != runtime.StatusSuccess {
			t.Fatalf("expected SUCCESS for %s, got %s", id, results[id].Status)
		}
	}
}

func TestParallelHandlerMaxParallel(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{"max_parallel": "2"}}
	g.AddNode(node)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("child_%d", i)
		g.AddNode(&model.Node{ID: id, Attrs: map[string]string{}})
		g.AddEdge(&model.Edge{From: "par", To: id, Order: i})
	}

	var maxConcurrent int64
	var currentConcurrent int64

	h := &ParallelHandler{
		ChildExecutor: func(_ context.Context, nodeID string) (runtime.Outcome, error) {
			cur := atomic.AddInt64(&currentConcurrent, 1)
			// Track max concurrency observed.
			for {
				old := atomic.LoadInt64(&maxConcurrent)
				if cur <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond) // hold slot briefly
			atomic.AddInt64(&currentConcurrent, -1)
			return runtime.SuccessOutcome(), nil
		},
	}

	pctx := runtime.NewContext()
	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil || o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s: %v", o.Status, err)
	}

	observed := atomic.LoadInt64(&maxConcurrent)
	if observed > 2 {
		t.Fatalf("max_parallel=2 but observed %d concurrent executions", observed)
	}
}

func TestParallelHandlerErrorPolicyFailFast(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{
		"error_policy": "fail_fast",
		"max_parallel": "1", // serialize to make behavior deterministic
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "a", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "b", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "c", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "par", To: "a", Order: 0})
	g.AddEdge(&model.Edge{From: "par", To: "b", Order: 1})
	g.AddEdge(&model.Edge{From: "par", To: "c", Order: 2})

	var executed int64
	h := &ParallelHandler{
		ChildExecutor: func(ctx context.Context, nodeID string) (runtime.Outcome, error) {
			atomic.AddInt64(&executed, 1)
			if nodeID == "a" {
				return runtime.FailOutcome("fail_a", runtime.FailureTransient), nil
			}
			// Check cancellation
			select {
			case <-ctx.Done():
				return runtime.FailOutcome("canceled", runtime.FailureCanceled), ctx.Err()
			default:
			}
			return runtime.SuccessOutcome(), nil
		},
	}

	pctx := runtime.NewContext()
	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatalf("expected FAIL with fail_fast policy, got %s", o.Status)
	}
}

func TestParallelHandlerErrorPolicyIgnore(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{
		"error_policy": "ignore",
		"join":         "wait_all",
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "a", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "b", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "par", To: "a", Order: 0})
	g.AddEdge(&model.Edge{From: "par", To: "b", Order: 1})

	h := &ParallelHandler{
		ChildExecutor: func(_ context.Context, nodeID string) (runtime.Outcome, error) {
			if nodeID == "a" {
				return runtime.FailOutcome("fail_a", runtime.FailureTransient), nil
			}
			return runtime.SuccessOutcome(), nil
		},
	}

	pctx := runtime.NewContext()
	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil {
		t.Fatal(err)
	}
	// With ignore policy, the failed branch is filtered before join evaluation.
	// wait_all on remaining {b: SUCCESS} should pass.
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected SUCCESS with ignore policy (failed branch filtered), got %s: %s", o.Status, o.FailureReason)
	}
}

func TestParallelHandlerErrorPolicyIgnoreAllFail(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "par", Attrs: map[string]string{"error_policy": "ignore"}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "a", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "par", To: "a", Order: 0})

	h := &ParallelHandler{
		ChildExecutor: func(_ context.Context, nodeID string) (runtime.Outcome, error) {
			return runtime.FailOutcome("fail", runtime.FailureTransient), nil
		},
	}

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected FAIL when all branches fail even with ignore policy")
	}
}

// --- Manager loop handler: supervision loop ---

func TestManagerLoopHandlerMaxCycles(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.max_cycles":    "5",
		"manager.poll_interval": "1ms",
		"manager.actions":       "observe,wait",
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	var cycles int
	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			cycles++
			return runtime.Outcome{Status: runtime.StatusRetry}, nil
		},
	}

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatalf("expected FAIL on max cycles, got %s", o.Status)
	}
	// Initial execution + 4 more cycles (cycle 1..4) = 5 total.
	if cycles != 5 {
		t.Fatalf("expected 5 child executions, got %d", cycles)
	}
}

func TestManagerLoopHandlerStopCondition(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.max_cycles":     "100",
		"manager.poll_interval":  "1ms",
		"manager.stop_condition": "context.done=true",
		"manager.actions":        "observe,wait",
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	pctx := runtime.NewContext()
	var cycles int
	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			cycles++
			if cycles == 3 {
				pctx.Set("done", "true")
			}
			return runtime.SuccessOutcome(), nil
		},
	}

	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected SUCCESS on stop condition, got %s: %s", o.Status, o.FailureReason)
	}
	if cycles > 10 {
		t.Fatalf("stop condition should have triggered early, got %d cycles", cycles)
	}
}

func TestManagerLoopHandlerChildStatus(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.max_cycles":    "100",
		"manager.poll_interval": "1ms",
		"manager.actions":       "observe,wait",
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	pctx := runtime.NewContext()
	var cycles int
	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			cycles++
			if cycles == 2 {
				pctx.Set("stack.child.status", "SUCCESS")
			}
			return runtime.SuccessOutcome(), nil
		},
	}

	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected SUCCESS when child status is SUCCESS, got %s", o.Status)
	}
}

func TestManagerLoopHandlerChildFail(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.max_cycles":    "100",
		"manager.poll_interval": "1ms",
		"manager.actions":       "observe,wait",
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	pctx := runtime.NewContext()
	var cycles int
	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			cycles++
			if cycles == 2 {
				pctx.Set("stack.child.status", "FAIL")
			}
			return runtime.SuccessOutcome(), nil
		},
	}

	o, err := h.Execute(context.Background(), node, pctx, g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatalf("expected FAIL when child status is FAIL, got %s", o.Status)
	}
}

func TestManagerLoopHandlerCancellation(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.max_cycles":    "1000",
		"manager.poll_interval": "100ms",
		"manager.actions":       "observe,wait",
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			return runtime.Outcome{Status: runtime.StatusRetry}, nil
		},
	}

	o, err := h.Execute(ctx, node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatalf("expected FAIL on cancellation, got %s", o.Status)
	}
}

func TestManagerLoopHandlerObserveOnly(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.max_cycles":    "5",
		"manager.poll_interval": "1ms",
		"manager.actions":       "observe", // no wait
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	var cycles int
	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			cycles++
			return runtime.Outcome{Status: runtime.StatusRetry}, nil
		},
	}

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatalf("expected FAIL on max cycles, got %s", o.Status)
	}
	if cycles != 5 {
		t.Fatalf("expected 5 cycles, got %d", cycles)
	}
}

func TestManagerLoopHandlerInvalidStopCondition(t *testing.T) {
	g := model.NewGraph("test")
	node := &model.Node{ID: "loop", Attrs: map[string]string{
		"manager.stop_condition": "&&", // empty clauses → parse error
	}}
	g.AddNode(node)
	g.AddNode(&model.Node{ID: "child", Attrs: map[string]string{}})
	g.AddEdge(&model.Edge{From: "loop", To: "child", Order: 0})

	h := &ManagerLoopHandler{
		ChildEngine: func(_ context.Context, children []string) (runtime.Outcome, error) {
			return runtime.SuccessOutcome(), nil
		},
	}

	o, err := h.Execute(context.Background(), node, runtime.NewContext(), g, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected FAIL for invalid stop condition")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"45s", 45 * time.Second},
		{"15m", 15 * time.Minute},
		{"2h", 2 * time.Hour},
		{"250ms", 250 * time.Millisecond},
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"", 5 * time.Second},
		{"invalid", 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDuration(tt.input, 5*time.Second)
			if got != tt.expected {
				t.Fatalf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStatusPriority(t *testing.T) {
	if statusPriority(runtime.StatusSuccess) >= statusPriority(runtime.StatusPartialSuccess) {
		t.Fatal("SUCCESS should have lower (better) priority than PARTIAL_SUCCESS")
	}
	if statusPriority(runtime.StatusPartialSuccess) >= statusPriority(runtime.StatusRetry) {
		t.Fatal("PARTIAL_SUCCESS should have lower priority than RETRY")
	}
	if statusPriority(runtime.StatusRetry) >= statusPriority(runtime.StatusFail) {
		t.Fatal("RETRY should have lower priority than FAIL")
	}
}

func TestCodingAgentHandlerStub(t *testing.T) {
	h := NewCodingAgentHandler(nil)
	node := &model.Node{ID: "agent1", Attrs: map[string]string{"prompt": "fix the bug"}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", o.Status, o.FailureReason)
	}
	if o.ContextUpdates["agent1.response"] != "(stub agent response)" {
		t.Error("expected stub response in context updates")
	}
}

func TestCodingAgentHandlerMissingPrompt(t *testing.T) {
	h := NewCodingAgentHandler(nil)
	node := &model.Node{ID: "agent2", Attrs: map[string]string{}}
	o, err := h.Execute(context.Background(), node, runtime.NewContext(), model.NewGraph("test"), "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != runtime.StatusFail {
		t.Fatal("expected fail for missing prompt")
	}
}

func TestCodingAgentHandlerRegistered(t *testing.T) {
	r := DefaultRegistry()
	node := &model.Node{ID: "a", Attrs: map[string]string{"type": "coding_agent"}}
	h, err := r.Lookup(node)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*CodingAgentHandler); !ok {
		t.Fatal("expected CodingAgentHandler for type=coding_agent")
	}
}

func TestLLMCodergenBackend(t *testing.T) {
	prov := &mockLLMProvider{response: "generated"}
	client := newMockClient(prov)
	backend := NewLLMCodergenBackend(client, "test-model")
	resp, err := backend.Generate(context.Background(), "write code", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != "generated" {
		t.Errorf("response = %q", resp)
	}
}

func TestRegistryLookup(t *testing.T) {
	r := DefaultRegistry()

	start := &model.Node{ID: "s", Attrs: map[string]string{"shape": "Mdiamond"}}
	h, err := r.Lookup(start)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*StartHandler); !ok {
		t.Fatal("expected StartHandler for Mdiamond")
	}

	cg := &model.Node{ID: "g", Attrs: map[string]string{"type": "codergen", "shape": "box"}}
	h, err = r.Lookup(cg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*CodergenHandler); !ok {
		t.Fatal("expected CodergenHandler for type=codergen")
	}

	unknown := &model.Node{ID: "x", Attrs: map[string]string{"shape": "star"}}
	_, err = r.Lookup(unknown)
	if err == nil {
		t.Fatal("expected error for unknown shape/type")
	}
}
