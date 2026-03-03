# Execution Environment

**Package**: `internal/agent/execenv`

Abstracts filesystem and process operations so agent tools can work against different backends (local filesystem, containers, remote hosts).

## Interface

```go
type Environment interface {
    ReadFile(ctx context.Context, path string) ([]byte, error)
    WriteFile(ctx context.Context, path string, data []byte, perm int) error
    Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error)
    Glob(ctx context.Context, pattern string) ([]string, error)
    WorkingDir() string
}

type ExecOpts struct {
    Dir     string  // working directory (empty = Environment.WorkingDir())
    Timeout int     // seconds (0 = default 10s)
}

type ExecResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
}
```

All tool implementations delegate to this interface rather than calling `os` directly. This makes tools testable and environment-agnostic.

## Local Implementation

`Local` implements `Environment` using the local filesystem and `os/exec`:

```go
env := execenv.NewLocal("/path/to/project")
```

### Path Resolution

Relative paths are resolved against the working directory. Absolute paths are used as-is. `WriteFile` creates parent directories automatically.

### Process Execution

Commands run via `sh -c` with process group isolation (`Setpgid: true`). Timeout handling follows the spec's three-phase shutdown:

```
SIGTERM (to process group)
  → wait up to 2 seconds
    → SIGKILL (to process group)
```

Default timeout is 10 seconds, maximum is 10 minutes.

### Environment Variable Filtering

Child processes inherit the parent's environment with sensitive variables stripped:

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `GEMINI_API_KEY`

This prevents agent-spawned commands from accidentally leaking API keys.

### Glob

Uses `filepath.Glob` and returns paths relative to the working directory.
