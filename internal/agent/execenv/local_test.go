package execenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalReadWriteFile(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)

	ctx := context.Background()
	data := []byte("hello world")
	if err := env.WriteFile(ctx, "test.txt", data, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := env.ReadFile(ctx, "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Errorf("got %q", string(got))
	}
}

func TestLocalWriteFileCreatesDir(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)

	ctx := context.Background()
	err := env.WriteFile(ctx, "sub/dir/file.txt", []byte("content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub/dir/file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Errorf("got %q", string(got))
	}
}

func TestLocalExec(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)

	result, err := env.Exec(context.Background(), "echo hello", ExecOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("stdout = %q", result.Stdout)
	}
}

func TestLocalExecNonZero(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)

	result, err := env.Exec(context.Background(), "exit 42", ExecOpts{})
	if err != nil {
		t.Fatal(err) // should not return error for non-zero exit
	}
	if result.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", result.ExitCode)
	}
}

func TestLocalExecTimeout(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)

	_, err := env.Exec(context.Background(), "sleep 30", ExecOpts{Timeout: 1})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestLocalGlob(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)

	ctx := context.Background()
	env.WriteFile(ctx, "a.go", []byte("package a"), 0644)
	env.WriteFile(ctx, "b.go", []byte("package b"), 0644)
	env.WriteFile(ctx, "c.txt", []byte("text"), 0644)

	matches, err := env.Glob(ctx, "*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Errorf("matches = %d, want 2", len(matches))
	}
}

func TestLocalWorkingDir(t *testing.T) {
	dir := t.TempDir()
	env := NewLocal(dir)
	if env.WorkingDir() != dir {
		t.Errorf("workdir = %q", env.WorkingDir())
	}
}

func TestFilteredEnv(t *testing.T) {
	// Set a filtered env var and verify it's removed.
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	env := filteredEnv()
	for _, e := range env {
		if len(e) > 18 && e[:18] == "ANTHROPIC_API_KEY=" {
			t.Error("ANTHROPIC_API_KEY should be filtered")
		}
	}
}
