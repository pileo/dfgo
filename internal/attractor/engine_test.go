package attractor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dfgo/internal/attractor/dot"
	"dfgo/internal/attractor/events"
	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
	"dfgo/internal/attractor/style"
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

func TestNodeStatusJSON(t *testing.T) {
	src := `digraph test {
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

	// Find the run directory and check for status.json in the work node dir.
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("expected run directory")
	}
	runDir := filepath.Join(dir, entries[0].Name())

	statusPath := filepath.Join(runDir, "work", "status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("expected status.json at %s: %v", statusPath, err)
	}

	var status nodeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("invalid status.json: %v", err)
	}
	if status.Outcome != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %s", status.Outcome)
	}
}

func TestEventsEmitted(t *testing.T) {
	src := `digraph test {
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

	engine := NewEngine(EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})

	// Run parse + validate + initialize to get event emitter
	g, err := engine.parse(src)
	if err != nil {
		t.Fatal(err)
	}
	engine.Graph = g
	if err := engine.applyStylesheet(g); err != nil {
		t.Fatal(err)
	}
	if err := engine.validate(g); err != nil {
		t.Fatal(err)
	}
	if err := engine.initialize(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var received []events.Event
	engine.Events.On(func(evt events.Event) {
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
	})

	if err := engine.execute(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if err := engine.finalize(); err != nil {
		t.Fatal(err)
	}

	// Wait for events to drain.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Should have PipelineStarted, at least one StageStarted/StageCompleted, PipelineCompleted.
	typeSet := make(map[events.Type]bool)
	for _, evt := range received {
		typeSet[evt.Type] = true
	}

	if !typeSet[events.PipelineStarted] {
		t.Error("expected PipelineStarted event")
	}
	if !typeSet[events.PipelineCompleted] {
		t.Error("expected PipelineCompleted event")
	}
	if !typeSet[events.StageStarted] {
		t.Error("expected StageStarted event")
	}
}

func TestStylesheetApplication(t *testing.T) {
	// Pipeline with a model_stylesheet that sets llm_model on box nodes.
	src := `digraph test {
		model_stylesheet="* { llm_model: gpt-4; }\n.box { temperature: 0.5; }"
		start [shape=Mdiamond]
		work [shape=box, type="capture"]
		explicit [shape=box, type="capture", llm_model="claude-3"]
		exit [shape=Msquare]
		start -> work -> explicit -> exit
	}`
	h := &contextCaptureHandler{}
	reg := handler.NewRegistry()
	reg.RegisterType("capture", h)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	engine := NewEngine(EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})

	g, err := engine.parse(src)
	if err != nil {
		t.Fatal(err)
	}
	engine.Graph = g

	if err := engine.applyStylesheet(g); err != nil {
		t.Fatal(err)
	}

	// "work" should get llm_model=gpt-4 and temperature=0.5.
	work := g.NodeByID("work")
	if work.Attrs["llm_model"] != "gpt-4" {
		t.Fatalf("expected llm_model=gpt-4 on work, got %s", work.Attrs["llm_model"])
	}
	if work.Attrs["temperature"] != "0.5" {
		t.Fatalf("expected temperature=0.5 on work, got %s", work.Attrs["temperature"])
	}

	// "explicit" should keep its explicit llm_model but get temperature from stylesheet.
	explicit := g.NodeByID("explicit")
	if explicit.Attrs["llm_model"] != "claude-3" {
		t.Fatalf("expected llm_model=claude-3 on explicit (not overridden), got %s", explicit.Attrs["llm_model"])
	}
	if explicit.Attrs["temperature"] != "0.5" {
		t.Fatalf("expected temperature=0.5 on explicit, got %s", explicit.Attrs["temperature"])
	}
}

func TestPreambleSetInContext(t *testing.T) {
	src := `digraph test {
		goal="test goal"
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
	if len(h.snapshots) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	// The preamble should have been set before handler execution.
	// Default fidelity is compact, so internal.preamble should be non-empty.
	preamble := h.snapshots[len(h.snapshots)-1]["internal.preamble"]
	if preamble == "" {
		t.Fatal("expected internal.preamble to be set")
	}
}

func TestContextLogs(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="logger"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	logHandler := &loggingTestHandler{}
	reg := handler.NewRegistry()
	reg.RegisterType("logger", logHandler)
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	dir := t.TempDir()

	engine := NewEngine(EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	err := engine.Run(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	logs := engine.PCtx.Logs()
	if len(logs) != 1 || logs[0] != "handler was here" {
		t.Fatalf("expected context log from handler, got %v", logs)
	}
}

// loggingTestHandler appends to context logs.
type loggingTestHandler struct{}

func (h *loggingTestHandler) Execute(_ context.Context, _ *model.Node, pctx *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	pctx.AppendLog("handler was here")
	return runtime.SuccessOutcome(), nil
}

func TestArtifactStoreAvailable(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`
	dir := t.TempDir()
	engine := NewEngine(EngineConfig{
		LogsDir: dir,
	})
	engine.Registry = handler.DefaultRegistry()
	err := engine.Run(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if engine.Artifacts == nil {
		t.Fatal("expected artifact store to be initialized")
	}
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

// ---------------------------------------------------------------------------
// Helper handlers for integration tests
// ---------------------------------------------------------------------------

// perNodeHandler dispatches to per-node-ID functions and counts calls.
type perNodeHandler struct {
	mu    sync.Mutex
	funcs map[string]func(int) runtime.Outcome
	calls map[string]int
}

func newPerNodeHandler(funcs map[string]func(int) runtime.Outcome) *perNodeHandler {
	return &perNodeHandler{funcs: funcs, calls: make(map[string]int)}
}

func (h *perNodeHandler) Execute(_ context.Context, node *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	h.mu.Lock()
	h.calls[node.ID]++
	n := h.calls[node.ID]
	h.mu.Unlock()

	if f, ok := h.funcs[node.ID]; ok {
		return f(n), nil
	}
	return runtime.SuccessOutcome(), nil
}

func (h *perNodeHandler) callCount(nodeID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[nodeID]
}

// suggestedNextHandler returns a success outcome with fixed SuggestedNextIDs.
type suggestedNextHandler struct {
	ids []string
}

func (h *suggestedNextHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.Outcome{
		Status:           runtime.StatusSuccess,
		SuggestedNextIDs: h.ids,
	}, nil
}

// preferredLabelHandler returns a success outcome with a fixed PreferredLabel.
type preferredLabelHandler struct {
	label string
}

func (h *preferredLabelHandler) Execute(_ context.Context, _ *model.Node, _ *runtime.Context, _ *model.Graph, _ string) (runtime.Outcome, error) {
	return runtime.SuccessOutcomeWithLabel(h.label), nil
}

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

func TestDefaultMaxRetryGraphAttr(t *testing.T) {
	// Graph sets default_max_retry=2. Node has no max_retries.
	// alwaysRetryHandler should get 1 initial + 2 retries = 3 total calls.
	src := `digraph test {
		default_max_retry="2"
		start [shape=Mdiamond]
		work [shape=box, type="test_retry", retry_policy="none"]
		exit [shape=Msquare]
		start -> work -> exit
	}`
	h := &countingRetryHandler{failFor: 100} // always retry
	reg := makeRegistryWithHandler("test_retry", h)
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	// Retries exhausted → FAIL → unconditional edge to exit
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 initial + 2 retries = 3 calls
	if got := h.count.Load(); got != 3 {
		t.Errorf("expected 3 calls (1 initial + 2 retries), got %d", got)
	}
}

func TestEdgeSelectionSuggestedNextIDs(t *testing.T) {
	// Handler returns SuggestedNextIDs=["target_b"].
	// Edge selector Step 3 should route to target_b, skipping target_a.
	src := `digraph test {
		start [shape=Mdiamond]
		router [shape=box, type="suggest"]
		target_a [shape=box, type="capture"]
		target_b [shape=box, type="capture"]
		exit [shape=Msquare]
		start -> router
		router -> target_a
		router -> target_b
		target_a -> exit
		target_b -> exit
	}`
	capture := &contextCaptureHandler{}
	reg := handler.NewRegistry()
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	reg.RegisterType("suggest", &suggestedNextHandler{ids: []string{"target_b"}})
	reg.RegisterType("capture", capture)
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	// capture should have exactly 1 snapshot (target_b visited, not target_a)
	if len(capture.snapshots) != 1 {
		t.Fatalf("expected exactly 1 capture, got %d", len(capture.snapshots))
	}
	if capture.snapshots[0]["current_node"] != "target_b" {
		t.Errorf("expected target_b visited, got %q", capture.snapshots[0]["current_node"])
	}
}

func TestEdgeSelectionPreferredLabel(t *testing.T) {
	// Handler returns PreferredLabel="b".
	// Edge selector Step 2 should route to target_b via labeled edge.
	src := `digraph test {
		start [shape=Mdiamond]
		router [shape=box, type="label_pick"]
		target_a [shape=box, type="capture"]
		target_b [shape=box, type="capture"]
		exit [shape=Msquare]
		start -> router
		router -> target_a [label="a"]
		router -> target_b [label="b"]
		target_a -> exit
		target_b -> exit
	}`
	capture := &contextCaptureHandler{}
	reg := handler.NewRegistry()
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	reg.RegisterType("label_pick", &preferredLabelHandler{label: "b"})
	reg.RegisterType("capture", capture)
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.snapshots) != 1 {
		t.Fatalf("expected exactly 1 capture, got %d", len(capture.snapshots))
	}
	if capture.snapshots[0]["current_node"] != "target_b" {
		t.Errorf("expected target_b visited, got %q", capture.snapshots[0]["current_node"])
	}
}

func TestFidelityModeTruncateInEngine(t *testing.T) {
	// Graph sets fidelity=truncate. Preamble should contain "Goal:" but not "Completed stages:".
	src := `digraph test {
		goal="test fidelity"
		fidelity="truncate"
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
	if len(h.snapshots) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	preamble := h.snapshots[0]["internal.preamble"]
	if !strings.Contains(preamble, "Goal:") {
		t.Error("truncate preamble should contain 'Goal:'")
	}
	if strings.Contains(preamble, "Completed stages:") {
		t.Error("truncate preamble should NOT contain 'Completed stages:'")
	}
}

func TestEventSequencing(t *testing.T) {
	// 2-node pipeline (start → work → exit).
	// Events must arrive: PipelineStarted before PipelineCompleted,
	// StageStarted before StageCompleted for each node.
	src := `digraph test {
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

	engine := NewEngine(EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})

	g, err := engine.parse(src)
	if err != nil {
		t.Fatal(err)
	}
	engine.Graph = g
	if err := engine.applyStylesheet(g); err != nil {
		t.Fatal(err)
	}
	if err := engine.validate(g); err != nil {
		t.Fatal(err)
	}
	if err := engine.initialize(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var received []events.Event
	engine.Events.On(func(evt events.Event) {
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
	})

	if err := engine.execute(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if err := engine.finalize(); err != nil {
		t.Fatal(err)
	}

	// Wait for events to drain.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Find indices of key events.
	indexOf := func(typ events.Type) int {
		for i, e := range received {
			if e.Type == typ {
				return i
			}
		}
		return -1
	}

	pipeStartIdx := indexOf(events.PipelineStarted)
	pipeCompleteIdx := indexOf(events.PipelineCompleted)
	stageStartIdx := indexOf(events.StageStarted)
	stageCompleteIdx := indexOf(events.StageCompleted)

	if pipeStartIdx < 0 {
		t.Fatal("missing PipelineStarted event")
	}
	if pipeCompleteIdx < 0 {
		t.Fatal("missing PipelineCompleted event")
	}
	if stageStartIdx < 0 {
		t.Fatal("missing StageStarted event")
	}
	if stageCompleteIdx < 0 {
		t.Fatal("missing StageCompleted event")
	}
	if pipeStartIdx >= pipeCompleteIdx {
		t.Errorf("PipelineStarted (%d) should come before PipelineCompleted (%d)", pipeStartIdx, pipeCompleteIdx)
	}
	if stageStartIdx >= stageCompleteIdx {
		t.Errorf("StageStarted (%d) should come before StageCompleted (%d)", stageStartIdx, stageCompleteIdx)
	}
	if pipeStartIdx >= stageStartIdx {
		t.Errorf("PipelineStarted (%d) should come before first StageStarted (%d)", pipeStartIdx, stageStartIdx)
	}
	if stageCompleteIdx >= pipeCompleteIdx {
		// Last stage complete should precede pipeline complete
		// Find the LAST StageCompleted
		lastStageComplete := -1
		for i, e := range received {
			if e.Type == events.StageCompleted {
				lastStageComplete = i
			}
		}
		if lastStageComplete >= pipeCompleteIdx {
			t.Errorf("last StageCompleted (%d) should come before PipelineCompleted (%d)", lastStageComplete, pipeCompleteIdx)
		}
	}
}

func TestParallelFanInContextKeys(t *testing.T) {
	// Pre-seed parallel.results in context, then run through a fan_in node
	// and capture context downstream to verify best_id/best_outcome are set.
	results := map[string]runtime.Outcome{
		"worker_a": {Status: runtime.StatusFail, FailureReason: "oops"},
		"worker_b": {Status: runtime.StatusSuccess},
	}
	resultsJSON, _ := json.Marshal(results)

	src := `digraph test {
		start [shape=Mdiamond]
		fan_in [shape=box, type="parallel.fan_in"]
		capture [shape=box, type="capture"]
		exit [shape=Msquare]
		start -> fan_in -> capture -> exit
	}`
	capture := &contextCaptureHandler{}
	reg := handler.NewRegistry()
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	reg.RegisterType("parallel.fan_in", &handler.FanInHandler{})
	reg.RegisterType("capture", capture)
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
		InitialContext: map[string]string{
			"parallel.results": string(resultsJSON),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.snapshots) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	snap := capture.snapshots[0]
	if snap["parallel.fan_in.best_id"] != "worker_b" {
		t.Errorf("expected best_id=worker_b, got %q", snap["parallel.fan_in.best_id"])
	}
	if snap["parallel.fan_in.best_outcome"] == "" {
		t.Error("expected parallel.fan_in.best_outcome to be set")
	}
}

func TestParallelExecutesBothChildren(t *testing.T) {
	// Verify that the parallel handler actually executes both children
	// and their context updates are available downstream via fan_in.
	src := `digraph test {
		start [shape=Mdiamond]
		fan_out [shape=component, type="parallel", join="wait_all"]
		worker_a [shape=box, type="mock"]
		worker_b [shape=box, type="mock"]
		fan_in [shape=tripleoctagon, type="parallel.fan_in"]
		capture [shape=box, type="capture"]
		exit [shape=Msquare]
		start -> fan_out
		fan_out -> worker_a
		fan_out -> worker_b
		worker_a -> fan_in
		worker_b -> fan_in
		fan_in -> capture -> exit
	}`
	mockH := newPerNodeHandler(map[string]func(int) runtime.Outcome{
		"worker_a": func(_ int) runtime.Outcome {
			return runtime.Outcome{
				Status:         runtime.StatusSuccess,
				ContextUpdates: map[string]string{"worker_a.done": "true"},
			}
		},
		"worker_b": func(_ int) runtime.Outcome {
			return runtime.Outcome{
				Status:         runtime.StatusSuccess,
				ContextUpdates: map[string]string{"worker_b.done": "true"},
			}
		},
	})
	capture := &contextCaptureHandler{}
	reg := handler.NewRegistry()
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	reg.RegisterType("mock", mockH)
	reg.RegisterType("parallel", handler.NewParallelHandler())
	reg.RegisterType("parallel.fan_in", &handler.FanInHandler{})
	reg.RegisterType("capture", capture)
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both workers must have been called.
	if got := mockH.callCount("worker_a"); got != 1 {
		t.Errorf("expected worker_a called 1 time, got %d", got)
	}
	if got := mockH.callCount("worker_b"); got != 1 {
		t.Errorf("expected worker_b called 1 time, got %d", got)
	}

	// Downstream capture node should see both context updates.
	if len(capture.snapshots) == 0 {
		t.Fatal("expected at least one capture snapshot")
	}
	snap := capture.snapshots[0]
	if snap["worker_a.done"] != "true" {
		t.Errorf("expected worker_a.done=true in context, got %q", snap["worker_a.done"])
	}
	if snap["worker_b.done"] != "true" {
		t.Errorf("expected worker_b.done=true in context, got %q", snap["worker_b.done"])
	}
}

func TestRetryDotFullChain(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "retry.dot"))
	if err != nil {
		t.Fatal(err)
	}

	// perNodeHandler behavior:
	//   setup: always succeed
	//   primary_work: FAIL once (goal gate retry → retry_target="setup"), then succeed
	//   secondary_work: always RETRY (allow_partial kicks in → PARTIAL_SUCCESS)
	//   validate: always succeed
	h := newPerNodeHandler(map[string]func(int) runtime.Outcome{
		"setup": func(call int) runtime.Outcome {
			return runtime.SuccessOutcome()
		},
		"primary_work": func(call int) runtime.Outcome {
			if call == 1 {
				return runtime.FailOutcome("first attempt fails", runtime.FailureTransient)
			}
			return runtime.SuccessOutcome()
		},
		"secondary_work": func(call int) runtime.Outcome {
			return runtime.RetryOutcome("always retry")
		},
		"validate": func(call int) runtime.Outcome {
			return runtime.SuccessOutcome()
		},
	})

	reg := handler.NewRegistry()
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	reg.RegisterType("codergen", h)
	dir := t.TempDir()

	err = RunPipeline(context.Background(), string(src), EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("expected pipeline to complete, got: %v", err)
	}

	// setup is called exactly once (goal gate retries primary_work in place, not via retry_target)
	if got := h.callCount("setup"); got != 1 {
		t.Errorf("expected setup called 1 time, got %d", got)
	}

	// primary_work: fail once → goal_gate retry in place → succeed = 2 calls
	if got := h.callCount("primary_work"); got != 2 {
		t.Errorf("expected primary_work called 2 times (fail + succeed), got %d", got)
	}

	// secondary_work: always retries → exhausts default_max_retry(2) → allow_partial → PARTIAL_SUCCESS
	// 1 initial + 2 retries = 3 calls
	if got := h.callCount("secondary_work"); got != 3 {
		t.Errorf("expected secondary_work called 3 times (1 + 2 retries), got %d", got)
	}

	// validate: succeeds immediately
	if got := h.callCount("validate"); got != 1 {
		t.Errorf("expected validate called 1 time, got %d", got)
	}
}

func TestFullFeaturesPipeline(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pipelines", "full_features.dot"))
	if err != nil {
		t.Fatal(err)
	}

	// Phase A: Parse-only assertions (stylesheet application).
	t.Run("stylesheet_attrs", func(t *testing.T) {
		g, err := dot.Parse(string(src))
		if err != nil {
			t.Fatal(err)
		}
		ssrc := g.Attrs["model_stylesheet"]
		if ssrc == "" {
			t.Fatal("expected model_stylesheet attr")
		}
		ss, err := style.ParseStylesheet(ssrc)
		if err != nil {
			t.Fatal(err)
		}
		ss.Apply(g)

		// worker_alpha should get llm_model=gpt-4 from * rule
		wa := g.NodeByID("worker_alpha")
		if wa.Attrs["llm_model"] != "gpt-4" {
			t.Errorf("worker_alpha.llm_model: expected gpt-4, got %q", wa.Attrs["llm_model"])
		}

		// worker_gamma has explicit llm_model=claude-3, should not be overridden
		wg := g.NodeByID("worker_gamma")
		if wg.Attrs["llm_model"] != "claude-3" {
			t.Errorf("worker_gamma.llm_model: expected claude-3 (explicit), got %q", wg.Attrs["llm_model"])
		}

		// reviewer should get temperature=0.2 and fidelity=truncate from #reviewer rule
		rv := g.NodeByID("reviewer")
		if rv.Attrs["temperature"] != "0.2" {
			t.Errorf("reviewer.temperature: expected 0.2, got %q", rv.Attrs["temperature"])
		}
		if rv.Attrs["fidelity"] != "truncate" {
			t.Errorf("reviewer.fidelity: expected truncate, got %q", rv.Attrs["fidelity"])
		}
	})

	// Phase B: Full run with mock handlers.
	// Reviewer returns "needs_fix" on call 1, "approved" on call 2.
	t.Run("full_run", func(t *testing.T) {
		reviewerCalls := 0
		h := newPerNodeHandler(map[string]func(int) runtime.Outcome{
			"reviewer": func(call int) runtime.Outcome {
				reviewerCalls++
				if call == 1 {
					return runtime.SuccessOutcomeWithLabel("needs_fix")
				}
				return runtime.SuccessOutcomeWithLabel("approved")
			},
		})

		reg := handler.NewRegistry()
		reg.RegisterShape("Mdiamond", &handler.StartHandler{})
		reg.RegisterType("codergen", h)
		// parallel and fan_in will be looked up by type; register stubs
		reg.RegisterType("parallel", &handler.StartHandler{})
		reg.RegisterType("parallel.fan_in", &handler.StartHandler{})
		dir := t.TempDir()

		err := RunPipeline(context.Background(), string(src), EngineConfig{
			LogsDir:  dir,
			Registry: reg,
		})
		if err != nil {
			t.Fatalf("expected pipeline to complete, got: %v", err)
		}

		if reviewerCalls != 2 {
			t.Errorf("expected reviewer called 2 times, got %d", reviewerCalls)
		}
	})
}

func TestManagerLoopIntegration(t *testing.T) {
	src := `digraph test {
		start [shape=Mdiamond]
		manager [shape=house, type="stack.manager_loop",
		         manager.max_cycles="10", manager.poll_interval="1ms"]
		worker [shape=box, type="codergen", prompt="do work"]
		exit [shape=Msquare]

		start -> manager -> worker -> exit
	}`

	// Custom codergen handler: sets stack.child.status=SUCCESS on the 3rd call.
	mockH := newPerNodeHandler(map[string]func(int) runtime.Outcome{
		"worker": func(call int) runtime.Outcome {
			if call == 3 {
				return runtime.Outcome{
					Status:         runtime.StatusSuccess,
					ContextUpdates: map[string]string{"stack.child.status": "SUCCESS"},
				}
			}
			return runtime.SuccessOutcome()
		},
	})

	reg := handler.NewRegistry()
	reg.RegisterShape("Mdiamond", &handler.StartHandler{})
	reg.RegisterType("stack.manager_loop", &handler.ManagerLoopHandler{})
	reg.RegisterType("codergen", mockH)
	dir := t.TempDir()

	err := RunPipeline(context.Background(), src, EngineConfig{
		LogsDir:  dir,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("expected pipeline to complete, got: %v", err)
	}

	// Worker should be called exactly 3 times by the manager loop.
	if got := mockH.callCount("worker"); got != 3 {
		t.Errorf("expected worker called 3 times, got %d", got)
	}
}
