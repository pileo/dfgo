package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dfgo/internal/agent/execenv"
)

type globTool struct{}

func NewGlob() Tool { return &globTool{} }

func (t *globTool) Name() string        { return "glob" }
func (t *globTool) Description() string { return "Find files matching a glob pattern" }
func (t *globTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.go, src/*.ts)"}
		},
		"required": ["pattern"]
	}`)
}

func (t *globTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Pattern == "" {
		return ErrorResult("pattern is required"), nil
	}

	matches, err := env.Glob(ctx, params.Pattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("glob error: %v", err)), nil
	}

	if len(matches) == 0 {
		return SuccessResult("glob", "No files matched"), nil
	}
	return SuccessResult("glob", strings.Join(matches, "\n")), nil
}
