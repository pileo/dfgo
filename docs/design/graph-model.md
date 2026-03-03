# Graph Model

**Package**: `internal/attractor/model`

The graph model holds the immutable, parsed representation of a pipeline. Once a DOT file is parsed into a `Graph`, nothing mutates it during execution.

## Structs

### Graph

```go
type Graph struct {
    Name  string
    Nodes []*Node
    Edges []*Edge
    Attrs map[string]string  // graph-level attributes (e.g., "goal", "fidelity")
}
```

Constructed via `NewGraph(name)`. Nodes are stored both in a slice (preserving order) and an internal `map[string]*Node` for O(1) ID lookup.

**Important graph-level attributes:**

| Attribute | Purpose | Default |
|---|---|---|
| `goal` | Pipeline goal, seeded into context | — |
| `fidelity` | Default fidelity mode | — |
| `default_max_retry` | Default max retries for nodes without explicit `max_retries` | `50` |
| `retry_target` | Graph-level fallback retry target node | — |
| `fallback_retry_target` | Graph-level last-resort retry target | — |
| `model_stylesheet` | CSS-like stylesheet for node attribute defaults | — |

### Node

```go
type Node struct {
    ID    string
    Attrs map[string]string
    Order int  // declaration order in the DOT file
}
```

Nodes are identified by their DOT ID (e.g., `start`, `do_work`). All properties come from the DOT attribute list and are stored as strings.

**Important attributes by convention:**

| Attribute | Purpose | Example |
|---|---|---|
| `shape` | Determines handler dispatch (fallback) | `Mdiamond`, `Msquare`, `box`, `diamond`, `hexagon`, `component`, `tripleoctagon`, `parallelogram`, `house` |
| `type` | Determines handler dispatch (primary) | `codergen`, `wait.human`, `conditional`, `parallel`, `parallel.fan_in`, `tool`, `stack.manager_loop` |
| `prompt` | LLM prompt text for codergen nodes | `"Write a function that..."` |
| `max_retries` | Max retry count for retry/goal-gate loops | `"3"` |
| `goal_gate` | Whether failure should trigger retry | `"true"` |
| `goal` | Human-readable goal for the node | `"Complete the task"` |
| `join` | Join policy for parallel nodes | `"wait_all"`, `"first_success"`, `"k_of_n"`, `"quorum"` |
| `tool_command` | Shell command for tool nodes (preferred) | `"go test ./..."` |
| `command` | Shell command for tool nodes (legacy alias) | `"go test ./..."` |
| `timeout` | Timeout in seconds for tool nodes | `"30"` |
| `fidelity` | Fidelity mode override | `"full"`, `"compact"`, `"summary:high"` |
| `allow_partial` | Accept partial success when retries exhausted | `"true"` |
| `retry_policy` | Backoff preset name | `"standard"`, `"none"`, `"patient"` |
| `retry_target` | Node to jump to on failure/retry | `"setup_step"` |
| `fallback_retry_target` | Fallback retry target | `"init_step"` |
| `class` | Comma-separated class list for stylesheet matching | `"agent, worker"` |

### Edge

```go
type Edge struct {
    From  string
    To    string
    Attrs map[string]string
    Order int  // declaration order
}
```

**Important edge attributes:**

| Attribute | Purpose | Example |
|---|---|---|
| `label` | Human-readable label, also used for preferred-label edge selection | `"yes"`, `"retry"` |
| `condition` | Condition expression for conditional routing | `"outcome=SUCCESS"` |
| `weight` | Priority weight for unconditional edge selection | `"10"` |
| `fidelity` | Fidelity mode override at edge level | `"full"` |

## Typed Attribute Accessors

Rather than storing typed fields, `Graph`, `Node`, and `Edge` all provide accessor methods that parse strings on demand:

```go
// Graph-level
g.IntAttr("default_max_retry", 50)           // returns int, or default if missing/unparseable
g.BoolAttr("verbose", false)                 // returns bool
g.StringAttr("goal", "")                     // returns string
g.FloatAttr("threshold", 0.5)               // returns float64
g.DurationAttr("timeout", 30*time.Second)    // returns time.Duration

// Node-level
node.IntAttr("max_retries", 0)
node.BoolAttr("goal_gate", false)
node.StringAttr("prompt", "")
node.FloatAttr("weight", 1.0)
node.DurationAttr("timeout", time.Minute)    // parses "900s", "15m", "2h", "250ms", "1d"

// Edge-level
edge.IntAttr("weight", 0)
edge.FloatAttr("weight", 0.0)
edge.StringAttr("label", "")
```

This keeps the model aligned with DOT's string-native format. Invalid values silently fall back to the default — no parse errors at the model layer.

### Duration Parsing

`DurationAttr` uses `model.ParseDuration()`, which extends Go's `time.ParseDuration` with support for `"d"` suffix (days, converted to hours). Examples: `"45s"`, `"15m"`, `"2h"`, `"250ms"`, `"1d"` (= 24h), `"7d"` (= 168h).

## Node Merging

When the parser encounters the same node ID multiple times (common in DOT — a node might be declared with attributes and then referenced in edge statements), attributes are merged:

```dot
A [shape=box]
A [label="Step A"]   // merges into existing node, adding label
A -> B               // references existing node, no new attrs
```

The `AddNode` method handles this: if a node with the same ID exists, new attributes are merged into it. The node's original `Order` is preserved.

## Graph Queries

```go
g.NodeByID("A")           // O(1) lookup
g.OutEdges("A")           // all edges from A, sorted by Order
g.InEdges("A")            // all edges to A, sorted by Order
g.Successors("A")         // IDs of nodes reachable from A
g.Predecessors("A")       // IDs of nodes with edges to A
g.StartNode()             // the node with shape="Mdiamond", or nil
g.ExitNodes()             // all nodes with shape="Msquare"
```

## Shape Conventions

Shapes follow Graphviz conventions with specific Attractor semantics:

| Shape | Role | Handler |
|---|---|---|
| `Mdiamond` | Pipeline entry point (exactly one) | `StartHandler` (no-op) |
| `Msquare` | Pipeline exit (one or more) | `ExitHandler` (no-op) |
| `box` | Generic stage (usually codergen) | Dispatched by `type` attr |
| `diamond` | Conditional routing | `ConditionalHandler` (pass-through) |
| `hexagon` | Human approval gate | `WaitHumanHandler` |
| `component` | Parallel fan-out | `ParallelHandler` |
| `tripleoctagon` | Parallel fan-in | `FanInHandler` |
| `parallelogram` | External tool | `ToolHandler` |
| `house` | Manager loop | `ManagerLoopHandler` |
