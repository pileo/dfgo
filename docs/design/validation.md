# Validation

**Package**: `internal/attractor/validate`

Lint-style validation runs after parsing and before execution, catching structural problems early.

## Framework

```go
type LintRule interface {
    Name() string
    Apply(g *model.Graph) []Diagnostic
}

type Diagnostic struct {
    Rule     string
    Severity Severity   // SeverityError or SeverityWarning
    NodeID   string     // optional
    Message  string
}
```

Errors block execution. Warnings are logged but don't prevent the pipeline from running.

```go
runner := validate.NewRunner()  // all built-in rules
diags := runner.Run(graph)
if validate.HasErrors(diags) {
    // abort
}
```

## Built-in Rules

### `start_node` (error)

Checks that exactly one node with `shape="Mdiamond"` exists. Zero or multiple start nodes are errors.

### `terminal_node` (error)

Checks that at least one node with `shape="Msquare"` exists. A pipeline with no exit is an error.

### `reachability` (error)

Walks the graph from the start node. Any node not reachable via outgoing edges is flagged as an error. Unreachable nodes are usually mistakes (orphaned after refactoring).

### `edge_target_exists` (error)

Verifies that both the source and target of every edge reference nodes that exist in the graph. This catches typos in node IDs.

### `condition_syntax` (error)

Parses every edge's `condition` attribute using `cond.Parse()`. Invalid condition syntax is flagged before the pipeline runs.

### `goal_gate_has_retry` (warning)

Nodes with `goal_gate="true"` should have a `max_retries` attribute, otherwise the goal gate has no retries and will fail immediately on first failure.

### `prompt_on_llm_nodes` (warning)

Nodes with `type="codergen"` should have a `prompt` attribute. Without one, the codergen handler will return a deterministic failure.

## Adding Custom Rules

Implement `LintRule` and add to the runner:

```go
runner := validate.NewRunner()
runner.Rules = append(runner.Rules, &MyCustomRule{})
```
