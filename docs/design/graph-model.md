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
| `command` | Shell command for tool nodes | `"go test ./..."` |
| `timeout` | Timeout in seconds for tool nodes | `"30"` |
| `fidelity` | Fidelity mode override | `"full"`, `"compact"`, `"summary:high"` |

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

Rather than storing typed fields, both `Node` and `Edge` provide accessor methods that parse strings on demand:

```go
node.IntAttr("max_retries", 0)    // returns int, or default if missing/unparseable
node.BoolAttr("goal_gate", false) // returns bool
node.StringAttr("prompt", "")     // returns string
node.FloatAttr("weight", 1.0)     // returns float64
```

This keeps the model aligned with DOT's string-native format. Invalid values silently fall back to the default — no parse errors at the model layer.

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
