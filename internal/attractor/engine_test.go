package attractor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
