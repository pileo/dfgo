# Handlers

**Package**: `internal/attractor/handler`

Handlers execute the logic for each pipeline stage. Every node type maps to a handler via the registry.

## Handler Interface

```go
type Handler interface {
    Execute(ctx context.Context, node *model.Node, pctx *runtime.Context,
            g *model.Graph, logsDir string) (runtime.Outcome, error)
}
```

Parameters:
- `ctx`: Go context for cancellation/timeout
- `node`: the graph node being executed
- `pctx`: mutable pipeline context (read/write state)
- `g`: the full graph (for handlers that need structural info, e.g., parallel)
- `logsDir`: directory path for this node's logs (empty if no run dir)

Returns an `Outcome` describing the result, or an error for unrecoverable failures.

## Optional Interfaces

Handlers can implement additional interfaces to opt into capabilities:

```go
// Adjusts behavior based on fidelity mode (e.g., less reasoning effort)
type FidelityAwareHandler interface {
    Handler
    SetFidelity(mode fidelity.Mode)
}

// Should only execute once even if the engine retries the node
type SingleExecutionHandler interface {
    Handler
    IsSingleExecution() bool
}
```

## Registry

The registry maps node types and shapes to handler instances. Lookup priority: `type` attribute first, then `shape` attribute.

```go
registry := handler.DefaultRegistry(
    handler.WithCodergenBackend(myBackend),
    handler.WithInterviewer(myInterviewer),
    handler.WithAgentSessionFactory(myFactory),
)
h, err := registry.Lookup(node)
```

You can also build a custom registry:

```go
r := handler.NewRegistry()
r.RegisterType("my_custom_type", &MyHandler{})
r.RegisterShape("star", &StarHandler{})
```

### Introspection

The registry exposes its registered keys for use by validation and tooling:

```go
registry.KnownTypes()  // []string{"codergen", "tool", "parallel", ...}
registry.KnownShapes() // []string{"Mdiamond", "Msquare", "diamond"}
```

The engine passes `KnownTypes()` to the validation runner so the `type_known` rule can warn about unrecognized node types.

## Built-in Handlers

### StartHandler

- **Shape**: `Mdiamond`
- **Behavior**: No-op, always returns `SUCCESS`
- **Purpose**: Entry point marker. Every pipeline must have exactly one.

### ExitHandler

- **Shape**: `Msquare`
- **Behavior**: No-op, always returns `SUCCESS`
- **Purpose**: Exit point marker. The engine stops when it reaches one.

### CodergenHandler

- **Type**: `codergen`
- **Shape**: typically `box`
- **Behavior**: Sends the node's `prompt` attribute to an LLM backend, stores the response in context as `{node_id}.response`
- **Backend**: `CodergenBackend` interface — currently a stub (returns placeholder text when no backend is configured)
- **Logging**: writes `prompt.txt` and `response.txt` to the node's logs directory
- **Failure**: returns `FAIL` with `FailureDeterministic` if no prompt attribute; `FailureTransient` on backend errors

```go
type CodergenBackend interface {
    Generate(ctx context.Context, prompt string, opts map[string]string) (string, error)
}
```

The `opts` map includes the node ID, graph goal, and all non-reserved node attributes.

### WaitHumanHandler

- **Type**: `wait.human`
- **Shape**: typically `hexagon`
- **Behavior**: Presents a question to a human via the `Interviewer` interface. If the node has 2+ outgoing edges with labels, presents them as multiple choice options. Otherwise asks yes/no.
- **Output**: sets `PreferredLabel` on the outcome to the human's answer, which drives edge selection (step 2)
- **Implements**: `SingleExecutionHandler` — should not re-prompt on retry

### ConditionalHandler

- **Type**: `conditional`
- **Shape**: `diamond`
- **Behavior**: No-op pass-through, always returns `SUCCESS`. The actual routing happens in edge selection — conditional edges on the diamond's outgoing edges determine the path.

### ParallelHandler

- **Type**: `parallel`
- **Shape**: `component`
- **Behavior**: Fans out to all child nodes (outgoing edge targets) concurrently, then joins results according to the `join` policy.
- **Current status**: stub mode — succeeds immediately without actual parallel execution. The `ChildExecutor` callback must be wired by the engine for real fan-out.

**Join Policies** (set via `join` attribute):

| Policy | Behavior |
|---|---|
| `wait_all` (default) | Succeeds only if all branches succeed |
| `first_success` | Succeeds if any branch succeeds |
| `k_of_n` | Succeeds if at least `k` branches succeed (set `k` attribute) |
| `quorum` | Succeeds if more than half the branches succeed |

### FanInHandler

- **Type**: `parallel.fan_in`
- **Shape**: `tripleoctagon`
- **Behavior**: Synchronization point for parallel branches. Currently a no-op pass-through — the engine treats it as a regular node.

### ToolHandler

- **Type**: `tool`
- **Shape**: typically `parallelogram`
- **Behavior**: Executes the node's shell command via `sh -c`. Captures stdout and stderr into context as `{node_id}.stdout` and `{node_id}.stderr`.
- **Command attribute**: reads `tool_command` first, falls back to `command` for backward compatibility
- **Timeout**: controlled by `timeout` attribute (default: 30 seconds)
- **Failure**: `FailureTransient` on non-zero exit or timeout; `FailureDeterministic` if no command attribute

### ManagerLoopHandler

- **Type**: `stack.manager_loop`
- **Shape**: `house`
- **Behavior**: Manages iterative refinement by executing child nodes in a loop until a goal is met.
- **Current status**: stub — succeeds immediately. The `ChildEngine` callback must be wired for real sub-pipeline execution.

### CodingAgentHandler

- **Type**: `coding_agent`
- **Behavior**: Runs a full autonomous agent session (agentic loop with tools) as a pipeline stage. Reads the node's `prompt` attribute, executes an agent `Session.Run()`, stores the final text in context as `{node_id}.response`.
- **Configuration**: `AgentSessionFactory` — injected via `WithAgentSessionFactory()` registry option or `EngineConfig.AgentSessionFactory`
- **Attributes**: `prompt` (required), `model`, `provider`, `max_rounds`
- **Stub mode**: if no session factory is configured, returns a placeholder response
- **See**: [coding-agent-handler.md](coding-agent-handler.md) for full details
