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

### Runner Options

`NewRunner` accepts variadic options to configure rule behavior:

```go
runner := validate.NewRunner(
    validate.WithKnownTypes(registry.KnownTypes()),  // enables type_known rule
)
```

The engine automatically passes known types from the handler registry when validating.

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

### `start_no_incoming` (error)

The start node (`shape="Mdiamond"`) must have no incoming edges. Edges pointing to the start node indicate a structural error in the pipeline.

### `exit_no_outgoing` (error)

Exit nodes (`shape="Msquare"`) must have no outgoing edges. An exit node with outgoing edges is a structural error — the pipeline should terminate at exit nodes.

### `stylesheet_syntax` (error)

If the graph has a `model_stylesheet` attribute, parses it via `style.ParseStylesheet()` and reports structural errors (unclosed braces, empty selectors).

### `type_known` (warning)

Checks that each node's `type` attribute matches a registered handler type. Only active when `WithKnownTypes` is provided to the runner (the engine does this automatically). Nodes without a `type` attribute are not checked.

### `fidelity_valid` (warning)

Validates `fidelity` attributes on nodes, edges, and the graph itself against `fidelity.Mode.Valid()`. Recognized modes: `full`, `compact`, `summary:high`, `summary:medium`, `summary:low`, `truncate`.

### `retry_target_exists` (warning)

If a node has `retry_target` or `fallback_retry_target` attributes, checks that the referenced node ID exists in the graph.

## Adding Custom Rules

Implement `LintRule` and add to the runner:

```go
runner := validate.NewRunner()
runner.Rules = append(runner.Rules, &MyCustomRule{})
```

## Test Fixtures

Invalid DOT fixtures in `testdata/pipelines/invalid/` exercise validation rules:

| Fixture | Rule triggered |
|---------|---------------|
| `no_start.dot` | `start_node` — missing start node |
| `no_terminal.dot` | `terminal_node` — missing exit node |
| `unreachable.dot` | `reachability` — orphaned nodes |

The `retry.dot` fixture triggers `goal_gate_has_retry` (warning) because the `validate` node has `goal_gate="true"` without an explicit `max_retries`. The `full_features.dot` fixture exercises `stylesheet_syntax` and `fidelity_valid` rules through its `model_stylesheet` and `fidelity` attributes.
