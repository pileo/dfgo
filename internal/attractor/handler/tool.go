package handler

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// ToolHandler executes an external tool via os/exec.
type ToolHandler struct{}

func (h *ToolHandler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	cmdStr := node.StringAttr("tool_command", node.StringAttr("command", ""))
	if cmdStr == "" {
		return runtime.FailOutcome("no command attribute on tool node", runtime.FailureDeterministic), nil
	}

	timeoutSec := node.IntAttr("timeout", 30)
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", cmdStr)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return runtime.FailOutcome("tool timed out", runtime.FailureTransient), nil
		}
		return runtime.FailOutcome(
			fmt.Sprintf("tool failed: %v\nstderr: %s", err, stderr.String()),
			runtime.FailureTransient,
		), nil
	}

	return runtime.Outcome{
		Status: runtime.StatusSuccess,
		ContextUpdates: map[string]string{
			node.ID + ".stdout": stdout.String(),
			node.ID + ".stderr": stderr.String(),
		},
	}, nil
}
