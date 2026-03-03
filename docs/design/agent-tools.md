# Agent Tools

**Package**: `internal/agent/tool`

Tools are the agent's interface to the filesystem and shell. Each tool validates its arguments, delegates to the execution environment, and returns a result that the LLM can act on.

## Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage   // JSON Schema
    Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error)
}
```

Tool errors are returned as `Result{IsError: true}` — passed to the model so it can recover. Go errors are reserved for truly unrecoverable situations.

```go
type Result struct {
    Content    string  // possibly truncated
    FullOutput string  // pre-truncation output
    IsError    bool
}
```

## Registry

The registry manages available tools and generates LLM SDK tool definitions:

```go
r := tool.DefaultRegistry()          // all 7 core tools
defs := r.ToolDefs()                 // []llm.ToolDef for the LLM request
result, err := r.Execute(ctx, env, "read_file", args)
```

Tools are listed in insertion order for deterministic output.

## Core Tools

### read_file

Reads a file's contents. Output truncated at 50K chars.

| Param | Type | Required |
|---|---|---|
| `path` | string | yes |

### write_file

Writes content to a file, creating parent directories if needed.

| Param | Type | Required |
|---|---|---|
| `path` | string | yes |
| `content` | string | yes |

### edit_file

Replaces an exact string match in a file. Fails if `old_string` matches zero or more than one time (must be unique).

| Param | Type | Required |
|---|---|---|
| `path` | string | yes |
| `old_string` | string | yes |
| `new_string` | string | yes |

### apply_patch

Applies a unified diff patch via `git apply`. Writes patch to a temp file, applies it, and cleans up.

| Param | Type | Required |
|---|---|---|
| `patch` | string | yes |

### shell

Executes a shell command via `sh -c`. Returns stdout, stderr, and exit code. Non-zero exit sets `IsError: true`.

| Param | Type | Required |
|---|---|---|
| `command` | string | yes |
| `timeout` | int | no (default 10s, max 600s) |

### grep

Searches file contents using ripgrep (falls back to grep). Output truncated at 20K chars / 200 lines.

| Param | Type | Required |
|---|---|---|
| `pattern` | string | yes |
| `path` | string | no (default: current dir) |
| `include` | string | no (glob filter, e.g. `*.go`) |

### glob

Finds files matching a glob pattern. Output truncated at 20K chars / 500 lines.

| Param | Type | Required |
|---|---|---|
| `pattern` | string | yes |

## Output Truncation

**Package**: `internal/agent/tool/truncate`

Large tool outputs are truncated using a two-phase middle-cut strategy that preserves the beginning and end:

1. **Line truncation**: if output exceeds `MaxLines`, keep `N/2` lines from head and tail, insert `... [X lines omitted] ...`
2. **Character truncation**: if output exceeds `MaxChars`, keep `N/2` chars from head and tail, insert `... [X characters omitted] ...`

Per-tool limits:

| Tool | MaxChars | MaxLines |
|---|---|---|
| `read_file` | 50,000 | — |
| `shell` | 30,000 | 256 |
| `grep` | 20,000 | 200 |
| `glob` | 20,000 | 500 |
| `edit_file` | 10,000 | — |
| `write_file` | 1,000 | — |

Unknown tools use fallback limits of 30K chars / 256 lines.
