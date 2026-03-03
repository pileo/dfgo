package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dfgo/internal/agent/execenv"
)

type applyPatchTool struct{}

func NewApplyPatch() Tool { return &applyPatchTool{} }

func (t *applyPatchTool) Name() string        { return "apply_patch" }
func (t *applyPatchTool) Description() string {
	return "Apply a unified diff patch to files"
}
func (t *applyPatchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {"type": "string", "description": "Unified diff patch content"}
		},
		"required": ["patch"]
	}`)
}

func (t *applyPatchTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Patch == "" {
		return ErrorResult("patch is required"), nil
	}

	// Write patch to temp file and apply via git apply.
	tmpPath := ".agent_patch.tmp"
	if err := env.WriteFile(ctx, tmpPath, []byte(params.Patch), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write patch file: %v", err)), nil
	}

	result, err := env.Exec(ctx, fmt.Sprintf("git apply %s && rm -f %s", tmpPath, tmpPath), execenv.ExecOpts{Timeout: 30})
	if err != nil {
		// Clean up temp file on error.
		env.Exec(ctx, fmt.Sprintf("rm -f %s", tmpPath), execenv.ExecOpts{})
		return ErrorResult(fmt.Sprintf("failed to apply patch: %v", err)), nil
	}
	if result.ExitCode != 0 {
		env.Exec(ctx, fmt.Sprintf("rm -f %s", tmpPath), execenv.ExecOpts{})
		return ErrorResult(fmt.Sprintf("patch failed: %s", result.Stderr)), nil
	}

	return SuccessResult("apply_patch", "Patch applied successfully"), nil
}
