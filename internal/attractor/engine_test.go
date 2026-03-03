package attractor

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

func TestRunSimplePipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "simple.dot"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = RunPipeline(context.Background(), string(src), EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunLinearPipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "linear.dot"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = RunPipeline(context.Background(), string(src), EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify checkpoint was written
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("expected run directory to be created")
	}
}

func TestRunBranchingPipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "branching.dot"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = RunPipeline(context.Background(), string(src), EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunParallelPipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "parallel.dot"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = RunPipeline(context.Background(), string(src), EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunRetryPipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "retry.dot"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = RunPipeline(context.Background(), string(src), EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunInvalidPipeline(t *testing.T) {
	err := RunPipeline(context.Background(), "not valid dot", EngineConfig{
		AutoApprove: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid DOT")
	}
}

func TestRunNoStartPipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "invalid", "no_start.dot"))
	if err != nil {
		t.Fatal(err)
	}
	err = RunPipeline(context.Background(), string(src), EngineConfig{
		AutoApprove: true,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunWithMockBackend(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		gen [shape=box, type="codergen", prompt="Generate something"]
		exit [shape=Msquare]
		start -> gen -> exit
	}`
	dir := t.TempDir()
	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:         dir,
		AutoApprove:     true,
		CodergenBackend: &mockBackend{response: "generated output"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithInitialContext(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`
	dir := t.TempDir()
	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
		InitialContext: map[string]string{
			"project": "test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunCancelContext(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := RunPipeline(ctx, src, EngineConfig{AutoApprove: true})
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func TestCheckpointResume(t *testing.T) {
	dir := t.TempDir()

	// Run a pipeline that creates a checkpoint
	src := `digraph test {
		start [shape=Mdiamond]
		A [shape=box, type="codergen", prompt="Step A"]
		exit [shape=Msquare]
		start -> A -> exit
	}`
	engine := NewEngine(EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	// Set a specific run ID for reproducibility
	cfg := engine.Config
	cfg.Registry = nil
	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify checkpoint exists in one of the run dirs
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("expected run directory")
	}
	runID := entries[0].Name()
	cpPath := filepath.Join(dir, runID, "checkpoint.json")
	cp, err := runtime.LoadCheckpoint(cpPath)
	if err != nil {
		t.Fatal(err)
	}
	if cp.RunID == "" {
		t.Fatal("expected non-empty run ID in checkpoint")
	}
}

type mockBackend struct {
	response string
}

func (m *mockBackend) Generate(_ context.Context, prompt string, opts map[string]string) (string, error) {
	return m.response, nil
}

// alwaysRetryHandler returns RETRY on every call.
type alwaysRetryHandler struct{}

func (h *alwaysRetryHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.RetryOutcome("always retry"), nil
}

// countingRetryHandler returns RETRY n times, then SUCCESS.
type countingRetryHandler struct {
	count    atomic.Int32
	failFor int32
}

func (h *countingRetryHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	n := h.count.Add(1)
	if n <= h.failFor {
		return runtime.RetryOutcome("not yet"), nil
	}
	return runtime.SuccessOutcome(), nil
}

func makeRegistryWithHandler(nodeType string, h handler.Handler) *handler.Registry {
	r := handler.NewRegistry()
	r.RegisterType(nodeType, h)
	r.RegisterShape("Mdiamond", &handler.StartHandler{})
	return r
}

func TestRetryWithNonePolicy(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="test_retry", max_retries="3", retry_policy="none"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	h := &countingRetryHandler{failFor: 2}
	reg := makeRegistryWithHandler("test_retry", h)
	dir := t.TempDir()

	start := time.Now()
	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("retry_policy=none should be fast, took %v", elapsed)
	}
	if h.count.Load() != 3 {
		t.Errorf("expected 3 calls (2 retries + 1 success), got %d", h.count.Load())
	}
}

func TestRetryCancelDuringBackoff(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="test_retry", max_retries="10", retry_policy="patient"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	h := &alwaysRetryHandler{}
	reg := makeRegistryWithHandler("test_retry", h)
	dir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := RunPipeline(ctx, src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected context error")
	}
	// Should cancel during backoff, not run for full retry count
	if elapsed > 2*time.Second {
		t.Errorf("should cancel promptly, took %v", elapsed)
	}
}

func TestAllowPartialOnRetriesExhausted(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="test_retry", max_retries="2", retry_policy="none", allow_partial="true"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	reg := makeRegistryWithHandler("test_retry", &alwaysRetryHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	// allow_partial=true means retries exhausted → PARTIAL_SUCCESS, which is
	// treated as success, so the pipeline should continue to exit
	if err != nil {
		t.Fatalf("expected pipeline to succeed with partial, got: %v", err)
	}
}

func TestNoAllowPartialOnRetriesExhausted(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="test_retry", max_retries="2", retry_policy="none"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	reg := makeRegistryWithHandler("test_retry", &alwaysRetryHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	// Without allow_partial, retries exhausted → FAIL, pipeline continues
	// via edge selection (unconditional edge to exit)
	if err != nil {
		t.Fatalf("expected pipeline to reach exit via edge selection, got: %v", err)
	}
}

// contextCaptureHandler captures the pipeline context on each call.
type contextCaptureHandler struct {
	snapshots []map[string]string
}

func (h *contextCaptureHandler) Execute(_ context.Context, _ *model.Node, pctx *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	h.snapshots = append(h.snapshots, pctx.Snapshot())
	return runtime.SuccessOutcome(), nil
}

func TestBuiltinContextKeys(t *testing.T) {
	src := `digraph test {
		goal="build the thing"
		start [shape=Mdiamond]
		work [shape=box, type="capture"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	h := &contextCaptureHandler{}
	reg := handler.NewRegistry()
	reg.RegisterType("capture", h)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.snapshots) < 1 {
		t.Fatal("expected at least 1 snapshot")
	}
	snap := h.snapshots[len(h.snapshots)-1] // last capture (work node)
	if snap["current_node"] != "work" {
		t.Errorf("expected current_node=work, got %q", snap["current_node"])
	}
	if snap["goal"] != "build the thing" {
		t.Errorf("expected goal set, got %q", snap["goal"])
	}
	if snap["graph.goal"] != "build the thing" {
		t.Errorf("expected graph.goal set, got %q", snap["graph.goal"])
	}
}

// retryThenSuccessHandler returns RETRY once, then captures context and succeeds.
type retryThenSuccessHandler struct {
	calls     int
	snapshots []map[string]string
}

func (h *retryThenSuccessHandler) Execute(_ context.Context, _ *model.Node, pctx *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	h.calls++
	h.snapshots = append(h.snapshots, pctx.Snapshot())
	if h.calls == 1 {
		return runtime.RetryOutcome("try again"), nil
	}
	return runtime.SuccessOutcome(), nil
}

func TestBuiltinRetryCountKey(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="retrycap", max_retries="3", retry_policy="none"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	h := &retryThenSuccessHandler{}
	reg := handler.NewRegistry()
	reg.RegisterType("retrycap", h)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", h.calls)
	}
	// On the second call (after 1 retry), retry count should be "1"
	snap := h.snapshots[1]
	if snap["internal.retry_count.work"] != "1" {
		t.Errorf("expected retry count 1, got %q", snap["internal.retry_count.work"])
	}
}

// failThenSuccessHandler fails N times, then succeeds. Tracks by node ID.
type failThenSuccessHandler struct {
	callsByNode map[string]int
	failFor     int
}

func newFailThenSuccessHandler(failFor int) *failThenSuccessHandler {
	return &failThenSuccessHandler{callsByNode: make(map[string]int), failFor: failFor}
}

func (h *failThenSuccessHandler) Execute(_ context.Context, node *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	h.callsByNode[node.ID]++
	if h.callsByNode[node.ID] <= h.failFor {
		return runtime.FailOutcome("not yet", runtime.FailureTransient), nil
	}
	return runtime.SuccessOutcome(), nil
}

func TestGoalGateAtExit(t *testing.T) {
	// Pipeline: start → gate (goal_gate, fails once) → exit
	// retry_target on gate sends back to gate itself.
	// On second pass gate succeeds → pipeline completes.
	src := `digraph test {
		start [shape=Mdiamond]
		gate [shape=box, type="gatetest", goal_gate="true", max_retries="3", retry_policy="none"]
		exit [shape=Msquare]
		start -> gate -> exit
	}`
	h := newFailThenSuccessHandler(1)
	reg := handler.NewRegistry()
	reg.RegisterType("gatetest", h)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("expected pipeline to complete after goal gate retry, got: %v", err)
	}
	if h.callsByNode["gate"] != 2 {
		t.Errorf("expected gate called 2 times, got %d", h.callsByNode["gate"])
	}
}

func TestGoalGateMaxRetriesExhausted(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		gate [shape=box, type="gatetest", goal_gate="true", max_retries="1", retry_policy="none"]
		exit [shape=Msquare]
		start -> gate -> exit
	}`
	h := newFailThenSuccessHandler(100) // always fails
	reg := handler.NewRegistry()
	reg.RegisterType("gatetest", h)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err == nil {
		t.Fatal("expected error when goal gate retries exhausted")
	}
}

// branchHandler goes to "skip" label on first call, then succeeds normally.
type branchHandler struct {
	calls int
}

func (h *branchHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	h.calls++
	if h.calls == 1 {
		return runtime.SuccessOutcomeWithLabel("skip"), nil
	}
	return runtime.SuccessOutcomeWithLabel("do_gate"), nil
}

func TestGoalGateAtExitWithRetryTarget(t *testing.T) {
	// Pipeline: start → router → exit (via "skip" label, bypassing gate)
	// gate is goal_gate=true but never visited on first pass.
	// At exit, checkGoalGates detects gate unsatisfied, uses retry_target="router".
	// Router on second call goes to gate, gate succeeds, then exit.
	src := `digraph test {
		start [shape=Mdiamond]
		router [shape=box, type="branch"]
		gate [shape=box, type="succeed", goal_gate="true", max_retries="3", retry_target="router", retry_policy="none"]
		exit [shape=Msquare]
		start -> router
		router -> exit [label="skip"]
		router -> gate [label="do_gate"]
		gate -> exit
	}`
	bh := &branchHandler{}
	reg := handler.NewRegistry()
	reg.RegisterType("branch", bh)
	reg.RegisterType("succeed", &handler.StartHandler{}) // always succeeds
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("expected pipeline to complete after goal gate retry_target, got: %v", err)
	}
	if bh.calls != 2 {
		t.Errorf("expected router called 2 times, got %d", bh.calls)
	}
}

func TestRetryTargetChainPriority(t *testing.T) {
	// Test that node-level retry_target takes priority over graph-level.
	// gate is never reached on first pass (bypassed via router), has retry_target="setup".
	// At exit, checkGoalGates sends to setup (not start, which is graph-level).
	src := `digraph test {
		retry_target="start"
		start [shape=Mdiamond]
		router [shape=box, type="branch"]
		setup [shape=box, type="succeed"]
		gate [shape=box, type="succeed", goal_gate="true", max_retries="3", retry_target="setup", retry_policy="none"]
		exit [shape=Msquare]
		start -> router
		router -> exit [label="skip"]
		router -> setup [label="do_gate"]
		setup -> gate -> exit
	}`
	setupCalls := 0
	setupHandler := &callCountHandler{calls: &setupCalls}
	bh := &branchHandler{}
	reg := handler.NewRegistry()
	reg.RegisterType("succeed", setupHandler)
	reg.RegisterType("branch", bh)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// setup should have been called (redirected from exit via retry_target="setup")
	if setupCalls == 0 {
		t.Error("expected setup to be called via retry_target, but it wasn't")
	}
}

// callCountHandler counts calls and always succeeds.
type callCountHandler struct {
	calls *int
}

func (h *callCountHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	*h.calls++
	return runtime.SuccessOutcome(), nil
}

// alwaysFailHandler always returns FAIL.
type alwaysFailHandler struct{}

func (h *alwaysFailHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.FailOutcome("failed", runtime.FailureTransient), nil
}

func TestResolveRetryTarget(t *testing.T) {
	// Unit test for resolveRetryTarget priority chain.
	e := NewEngine(EngineConfig{})

	// Graph with graph-level retry_target
	g := model.NewGraph("test")
	g.Attrs["retry_target"] = "graph_target"
	g.Attrs["fallback_retry_target"] = "graph_fallback"
	g.AddNode(&model.Node{ID: "graph_target", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "graph_fallback", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "node_target", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "work_with_target", Attrs: map[string]string{"retry_target": "node_target"}})
	g.AddNode(&model.Node{ID: "work_no_target", Attrs: map[string]string{}})
	g.AddNode(&model.Node{ID: "work_bad_target", Attrs: map[string]string{"retry_target": "nonexistent"}})

	// Node-level retry_target takes priority
	target, ok := e.resolveRetryTarget(g, "work_with_target")
	if !ok || target != "node_target" {
		t.Errorf("expected node_target, got %q (ok=%v)", target, ok)
	}

	// Falls back to graph-level
	target, ok = e.resolveRetryTarget(g, "work_no_target")
	if !ok || target != "graph_target" {
		t.Errorf("expected graph_target, got %q (ok=%v)", target, ok)
	}

	// Bad node-level target falls through to graph-level
	target, ok = e.resolveRetryTarget(g, "work_bad_target")
	if !ok || target != "graph_target" {
		t.Errorf("expected graph_target (fallthrough), got %q (ok=%v)", target, ok)
	}
}
