package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dfgo/internal/agent/execenv"
)

type writeFileTool struct{}

func NewWriteFile() Tool { return &writeFileTool{} }

func (t *writeFileTool) Name() string        { return "write_file" }
func (t *writeFileTool) Description() string  { return "Write content to a file, creating it if it doesn't exist" }
func (t *writeFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Path to the file"},
			"content": {"type": "string", "description": "Content to write"}
		},
		"required": ["path", "content"]
	}`)
}

func (t *writeFileTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Path == "" {
		return ErrorResult("path is required"), nil
	}

	if err := env.WriteFile(ctx, params.Path, []byte(params.Content), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err)), nil
	}
	return SuccessResult("write_file", fmt.Sprintf("Wrote %d bytes to %s", len(params.Content), params.Path)), nil
}
