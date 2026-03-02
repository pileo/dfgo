# Transforms

**Package**: `internal/attractor/transform`

Pre-processing transforms that modify strings (typically prompts) before they're passed to handlers.

## Interface

```go
type Transform interface {
    Name() string
    Apply(input string, vars map[string]string) string
}
```

## Runner

Applies a chain of transforms in sequence:

```go
runner := transform.NewRunner()  // includes default transforms
result := runner.Apply("Goal: $goal", map[string]string{"goal": "build something"})
// result: "Goal: build something"
```

## Built-in: VariableExpand

Replaces `$varname` and `${varname}` references with values from a variable map.

### Patterns

| Syntax | Example | Notes |
|---|---|---|
| `$name` | `$goal` | Simple variable, ends at non-identifier char |
| `${name}` | `${step_a.response}` | Braced variable, supports any characters |

Identifiers can contain letters, digits, underscores, and dots (e.g., `$step_a.response`).

### Unresolved Variables

Variables not found in the map are left as-is:

```go
transform.Apply("Hello $missing", map[string]string{})
// "Hello $missing"
```

This is intentional — it avoids silent data loss and makes missing variables visible.

## Current Status

The transform runner is instantiated by the engine but **not yet called** during execution. The planned integration point is in `executeNode`, where prompts would be expanded against the pipeline context snapshot before being passed to the handler.
