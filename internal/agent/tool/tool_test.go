package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"dfgo/internal/agent/execenv"
)

func testEnv(t *testing.T) execenv.Environment {
	t.Helper()
	return execenv.NewLocal(t.TempDir())
}

func TestReadFile(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	env.WriteFile(ctx, "test.txt", []byte("hello world"), 0644)

	tool := NewReadFile()
	result, err := tool.Execute(ctx, env, json.RawMessage(`{"path":"test.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "hello world" {
		t.Errorf("content = %q", result.Content)
	}
}

func TestReadFileMissing(t *testing.T) {
	env := testEnv(t)
	tool := NewReadFile()
	result, err := tool.Execute(context.Background(), env, json.RawMessage(`{"path":"nonexistent.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing file")
	}
}

func TestWriteFile(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	tool := NewWriteFile()
	result, err := tool.Execute(ctx, env, json.RawMessage(`{"path":"out.txt","content":"written"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, _ := env.ReadFile(ctx, "out.txt")
	if string(data) != "written" {
		t.Errorf("file content = %q", string(data))
	}
}

func TestEditFile(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	env.WriteFile(ctx, "edit.txt", []byte("foo bar baz"), 0644)

	tool := NewEditFile()
	result, err := tool.Execute(ctx, env, json.RawMessage(`{"path":"edit.txt","old_string":"bar","new_string":"qux"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, _ := env.ReadFile(ctx, "edit.txt")
	if string(data) != "foo qux baz" {
		t.Errorf("file content = %q", string(data))
	}
}

func TestEditFileNotFound(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	env.WriteFile(ctx, "edit.txt", []byte("foo bar baz"), 0644)

	tool := NewEditFile()
	result, _ := tool.Execute(ctx, env, json.RawMessage(`{"path":"edit.txt","old_string":"xyz","new_string":"qux"}`))
	if !result.IsError {
		t.Error("expected error for non-matching old_string")
	}
}

func TestEditFileNonUnique(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	env.WriteFile(ctx, "edit.txt", []byte("foo foo foo"), 0644)

	tool := NewEditFile()
	result, _ := tool.Execute(ctx, env, json.RawMessage(`{"path":"edit.txt","old_string":"foo","new_string":"bar"}`))
	if !result.IsError {
		t.Error("expected error for non-unique match")
	}
}

func TestShell(t *testing.T) {
	env := testEnv(t)
	tool := NewShell()
	result, err := tool.Execute(context.Background(), env, json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "hello\n" {
		t.Errorf("content = %q", result.Content)
	}
}

func TestShellNonZero(t *testing.T) {
	env := testEnv(t)
	tool := NewShell()
	result, _ := tool.Execute(context.Background(), env, json.RawMessage(`{"command":"exit 1"}`))
	if !result.IsError {
		t.Error("expected error for non-zero exit")
	}
}

func TestGlob(t *testing.T) {
	dir := t.TempDir()
	env := execenv.NewLocal(dir)
	ctx := context.Background()

	// Create test files.
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0644)

	tool := NewGlob()
	result, err := tool.Execute(ctx, env, json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	// Should find 2 .go files.
	if result.Content == "No files matched" {
		t.Error("expected matches")
	}
}

func TestRegistryDefault(t *testing.T) {
	r := DefaultRegistry()
	tools := r.All()
	if len(tools) != 7 {
		t.Errorf("tools = %d, want 7", len(tools))
	}

	// Check all tools are present.
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	expected := []string{"read_file", "write_file", "edit_file", "apply_patch", "shell", "grep", "glob"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestRegistryToolDefs(t *testing.T) {
	r := DefaultRegistry()
	defs := r.ToolDefs()
	if len(defs) != 7 {
		t.Errorf("defs = %d, want 7", len(defs))
	}
	for _, d := range defs {
		if d.Name == "" {
			t.Error("empty tool name in def")
		}
		if len(d.Parameters) == 0 {
			t.Errorf("empty parameters for %s", d.Name)
		}
	}
}

func TestRegistryUnknownTool(t *testing.T) {
	r := DefaultRegistry()
	result, err := r.Execute(context.Background(), testEnv(t), "nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for unknown tool")
	}
}
