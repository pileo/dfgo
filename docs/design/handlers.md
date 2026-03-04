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
- **Behavior**: Fans out to all child nodes (outgoing edge targets) concurrently, then joins results according to the `join` policy. The `ChildExecutor` callback is set by the engine at runtime to execute each branch. In stub mode (no executor), succeeds immediately.

**Attributes:**

| Attribute | Default | Description |
|---|---|---|
| `join` | `wait_all` | Join policy (see below) |
| `error_policy` | `continue` | How to handle branch failures (see below) |
| `max_parallel` | `4` | Maximum concurrent branch goroutines (channel-based semaphore) |
| `k` | `0` | Required successes for `k_of_n` policy |

**Join Policies** (set via `join` attribute):

| Policy | Behavior |
|---|---|
| `wait_all` (default) | Succeeds only if all branches succeed |
| `first_success` | Succeeds if any branch succeeds |
| `k_of_n` | Succeeds if at least `k` branches succeed (set `k` attribute) |
| `quorum` | Succeeds if more than half the branches succeed |

**Error Policies** (set via `error_policy` attribute):

| Policy | Behavior |
|---|---|
| `continue` (default) | Run all branches regardless of failures |
| `fail_fast` | Cancel remaining branches on first failure (via context cancellation) |
| `ignore` | Filter out failed branches before join evaluation |

**Context output:** After execution, stores all branch outcomes as JSON in `parallel.results` for downstream fan-in consumption.

### FanInHandler

- **Type**: `parallel.fan_in`
- **Shape**: `tripleoctagon`
- **Behavior**: Consolidates results from parallel branches using heuristic ranking.

**Algorithm:**
1. Reads `parallel.results` from pipeline context (JSON map of nodeID → outcome, written by ParallelHandler)
2. Ranks candidates by status priority: SUCCESS (0) > PARTIAL_SUCCESS (1) > RETRY (2) > FAIL (3), with alphabetical node ID as tiebreaker
3. Writes `parallel.fan_in.best_id` and `parallel.fan_in.best_outcome` (JSON) to pipeline context
4. Returns SUCCESS if any candidate succeeded, FAIL if all candidates failed

If no `parallel.results` key exists in context (e.g., node used as a plain synchronization point), passes through with SUCCESS.

`TestParallelFanInContextKeys` in `engine_test.go` verifies the full pipeline: pre-seeded `parallel.results` → fan-in ranking → downstream node reads `parallel.fan_in.best_id` and `parallel.fan_in.best_outcome` from context.

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
- **Behavior**: Manages iterative refinement by executing child nodes in a supervision loop until a stop condition is met or max cycles are exhausted. The `ChildEngine` callback is set by the engine to execute sub-pipelines. In stub mode (no engine), succeeds immediately.

**Attributes:**

| Attribute | Default | Description |
|---|---|---|
| `manager.poll_interval` | `45s` | Duration between observation cycles (supports `ms`, `s`, `m`, `h`, `d` suffixes) |
| `manager.max_cycles` | `1000` | Maximum loop iterations before failing |
| `manager.stop_condition` | (none) | Condition expression evaluated via `cond.Eval` each cycle |
| `manager.actions` | `observe,wait` | Comma-separated actions per cycle |

**Loop behavior:**
1. Executes child pipeline once to start
2. Each subsequent cycle: observe → check stop conditions → wait → re-execute
3. **Observe**: reads `stack.child.status` from context; returns SUCCESS if child completed, FAIL if child failed; evaluates custom `stop_condition` expression
4. **Wait**: sleeps for `poll_interval` with context cancellation support (`select` on `time.After` and `ctx.Done()`)
5. Terminates with FAIL if `max_cycles` is exceeded

### CodingAgentHandler

- **Type**: `coding_agent`
- **Behavior**: Runs a full autonomous agent session (agentic loop with tools) as a pipeline stage. Reads the node's `prompt` attribute, executes an agent `Session.Run()`, stores the final text in context as `{node_id}.response`.
- **Configuration**: `AgentSessionFactory` — injected via `WithAgentSessionFactory()` registry option or `EngineConfig.AgentSessionFactory`
- **Attributes**: `prompt` (required), `model`, `provider`, `max_rounds`
- **Stub mode**: if no session factory is configured, returns a placeholder response
- **See**: [coding-agent-handler.md](coding-agent-handler.md) for full details

## Simulation

The `simulate` package (`internal/attractor/simulate`) provides a `Handler` that replaces `codergen` and `coding_agent` types with deterministic, rule-based responses. This enables full pipeline testing without LLM API calls.

```go
reg := simulate.BuildRegistry(simConfig)
```

`BuildRegistry` starts from `DefaultRegistry` and overrides `codergen` and `coding_agent` with the simulation handler. All other handlers (start, exit, conditional, parallel, etc.) remain unchanged.

See [simulation.md](simulation.md) for full details on config format and rule matching.
