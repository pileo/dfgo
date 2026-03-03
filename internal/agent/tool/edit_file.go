package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dfgo/internal/agent/execenv"
)

type editFileTool struct{}

func NewEditFile() Tool { return &editFileTool{} }

func (t *editFileTool) Name() string        { return "edit_file" }
func (t *editFileTool) Description() string {
	return "Edit a file by replacing an exact string match with new content"
}
func (t *editFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Path to the file"},
			"old_string": {"type": "string", "description": "Exact string to find and replace"},
			"new_string": {"type": "string", "description": "Replacement string"}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *editFileTool) Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
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

	content := string(data)
	count := strings.Count(content, params.OldString)
	if count == 0 {
		return ErrorResult("old_string not found in file"), nil
	}
	if count > 1 {
		return ErrorResult(fmt.Sprintf("old_string matches %d times; must be unique", count)), nil
	}

	newContent := strings.Replace(content, params.OldString, params.NewString, 1)
	if err := env.WriteFile(ctx, params.Path, []byte(newContent), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err)), nil
	}

	return SuccessResult("edit_file", fmt.Sprintf("Edited %s", params.Path)), nil
}
