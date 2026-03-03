package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dfgo/internal/agent/execenv"
)

type grepTool struct{}

func NewGrep() Tool { return &grepTool{} }

func (t *grepTool) Name() string        { return "grep" }
func (t *grepTool) Description() string { return "Search file contents using grep/ripgrep" }
func (t *grepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Regex pattern to search for"},
			"path": {"type": "string", "description": "Directory or file to search (default: current dir)"},
			"include": {"type": "string", "description": "Glob pattern to filter files (e.g. *.go)"}
		},
		"required": ["pattern"]
	}`)
}

func (t *grepTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Pattern == "" {
		return ErrorResult("pattern is required"), nil
	}

	// Build grep command, preferring ripgrep if available.
	cmd := fmt.Sprintf("rg -n --no-heading %q", params.Pattern)
	if params.Include != "" {
		cmd += fmt.Sprintf(" --glob %q", params.Include)
	}
	if params.Path != "" {
		cmd += " " + params.Path
	} else {
		cmd += " ."
	}
	// Fallback to grep if rg not found.
	cmd += " 2>/dev/null || grep -rn " + fmt.Sprintf("%q", params.Pattern)
	if params.Include != "" {
		cmd += fmt.Sprintf(" --include=%q", params.Include)
	}
	if params.Path != "" {
		cmd += " " + params.Path
	} else {
		cmd += " ."
	}

	result, err := env.Exec(ctx, cmd, execenv.ExecOpts{Timeout: 30})
	if err != nil {
		return ErrorResult(fmt.Sprintf("grep error: %v", err)), nil
	}

	output := result.Stdout
	if output == "" {
		return SuccessResult("grep", "No matches found"), nil
	}
	return SuccessResult("grep", output), nil
}
