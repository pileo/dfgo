package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dfgo/internal/agent/execenv"
)

type readFileTool struct{}

func NewReadFile() Tool { return &readFileTool{} }

func (t *readFileTool) Name() string        { return "read_file" }
func (t *readFileTool) Description() string  { return "Read the contents of a file" }
func (t *readFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Path to the file to read"}
		},
		"required": ["path"]
	}`)
}

func (t *readFileTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Path == "" {
		return ErrorResult("path is required"), nil
	}

	data, err := env.ReadFile(ctx, params.Path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}
	return SuccessResult("read_file", string(data)), nil
}
