package runmgr

import (
	"context"
	"testing"
	"time"

	"dfgo/internal/attractor"
)

const simplePipeline = `digraph simple {
	start [shape=Mdiamond]
	end [shape=Msquare]
	start -> end
}`

func TestSubmitAndComplete(t *testing.T) {
	cfg := ManagerConfig{
		BaseCfg: attractor.EngineConfig{
			LogsDir:     t.TempDir(),
			AutoApprove: true,
		},
	}
	mgr := NewRunManager(cfg)

	id, err := mgr.Submit(context.Background(), SubmitOptions{
		DOTSource:   simplePipeline,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty run ID")
	}

	// Wait for completion.
	run := mgr.Get(id)
	if run == nil {
		t.Fatal("run not found after submit")
	}
	run.wg.Wait()

	snap := run.Snapshot()
	if snap.Status != StatusCompleted {
		t.Errorf("expected completed, got %s (error: %s)", snap.Status, snap.Error)
	}
}

func TestSubmitInvalidDOT(t *testing.T) {
	cfg := ManagerConfig{
		BaseCfg: attractor.EngineConfig{
			LogsDir:     t.TempDir(),
			AutoApprove: true,
		},
	}
	mgr := NewRunManager(cfg)

	_, err := mgr.Submit(context.Background(), SubmitOptions{
		DOTSource: "not valid dot",
	})
	if err == nil {
		t.Fatal("expected error for invalid DOT")
	}
}

func TestCancel(t *testing.T) {
	// Use a pipeline that will block on a human gate.
	blockingPipeline := `digraph blocking {
		start [shape=Mdiamond]
		gate [type="wait.human" prompt="Approve?"]
		end [shape=Msquare]
		start -> gate -> end
	}`

	cfg := ManagerConfig{
		BaseCfg: attractor.EngineConfig{
			LogsDir: t.TempDir(),
		},
	}
	mgr := NewRunManager(cfg)

	id, err := mgr.Submit(context.Background(), SubmitOptions{
		DOTSource: blockingPipeline,
	})
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}

	// Give it a moment to start executing.
	time.Sleep(50 * time.Millisecond)

	if err := mgr.Cancel(id); err != nil {
		t.Fatalf("Cancel error: %v", err)
	}

	run := mgr.Get(id)
	run.wg.Wait()

	snap := run.Snapshot()
	if snap.Status != StatusCanceled && snap.Status != StatusFailed {
		t.Errorf("expected canceled or failed, got %s", snap.Status)
	}
}

func TestShutdown(t *testing.T) {
	cfg := ManagerConfig{
		BaseCfg: attractor.EngineConfig{
			LogsDir:     t.TempDir(),
			AutoApprove: true,
		},
	}
	mgr := NewRunManager(cfg)

	_, err := mgr.Submit(context.Background(), SubmitOptions{
		DOTSource:   simplePipeline,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Shutdown(ctx)
}
