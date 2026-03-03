// Package execenv defines the execution environment interface for agent tools.
package execenv

import "context"

// Environment abstracts filesystem and process operations for agent tools.
type Environment interface {
	// ReadFile reads the contents of a file.
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// WriteFile writes data to a file, creating it if necessary.
	WriteFile(ctx context.Context, path string, data []byte, perm int) error

	// Exec runs a shell command and returns its combined output.
	Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error)

	// Glob returns file paths matching a pattern.
	Glob(ctx context.Context, pattern string) ([]string, error)

	// WorkingDir returns the current working directory.
	WorkingDir() string
}

// ExecOpts configures command execution.
type ExecOpts struct {
	Dir     string // working directory (empty = Environment.WorkingDir())
	Timeout int    // seconds (0 = default)
}

// ExecResult holds the result of a command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}
