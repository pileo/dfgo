package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dfgo/internal/agent/execenv"
)

type shellTool struct{}

func NewShell() Tool { return &shellTool{} }

func (t *shellTool) Name() string        { return "shell" }
func (t *shellTool) Description() string { return "Execute a shell command" }
func (t *shellTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Shell command to execute"},
			"timeout": {"type": "integer", "description": "Timeout in seconds (default 10, max 600)"}
		},
		"required": ["command"]
	}`)
}

func (t *shellTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Command == "" {
		return ErrorResult("command is required"), nil
	}

	result, err := env.Exec(ctx, params.Command, execenv.ExecOpts{Timeout: params.Timeout})
	if err != nil {
		return ErrorResult(fmt.Sprintf("command error: %v\nstderr: %s", err, result.Stderr)), nil
	}

	var output strings.Builder
	if result.Stdout != "" {
		output.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.WriteString(result.Stderr)
	}
	if result.ExitCode != 0 {
		output.WriteString(fmt.Sprintf("\nExit code: %d", result.ExitCode))
	}

	r := SuccessResult("shell", output.String())
	if result.ExitCode != 0 {
		r.IsError = true
	}
	return r, nil
}
