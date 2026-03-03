# Engine

**Package**: `internal/attractor`

The engine orchestrates pipeline execution through a 5-phase lifecycle.

## Configuration

```go
type EngineConfig struct {
    Registry        *handler.Registry     // nil = use DefaultRegistry
    LogsDir         string                // default: "runs"
    ResumeRunID     string                // resume a previous run
    InitialContext  map[string]string     // seed context
    CodergenBackend handler.CodergenBackend
    AgentSessionFactory handler.AgentSessionFactory
    Interviewer     interviewer.Interviewer
    AutoApprove     bool                  // use AutoApprove interviewer
    Artifacts       *artifact.Store       // nil = auto-created from run dir
}
```

## Quick Start

```go
err := attractor.RunPipeline(ctx, dotSource, attractor.EngineConfig{
    AutoApprove: true,
    LogsDir:     "runs",
})
```

`RunPipeline` is a convenience wrapper that builds the registry from config and calls `Engine.Run()`.

## Phase 1: Parse

Feeds DOT source to the hand-rolled parser. Produces an immutable `model.Graph`.

Fails on: malformed DOT syntax, unterminated strings, unexpected tokens.

### Stylesheet Transform

After parsing but before validation, the engine applies the model stylesheet if the graph has a `model_stylesheet` attribute. The stylesheet is parsed via `style.ParseStylesheet()` and applied via `ss.Apply(g)`, which sets properties on nodes only when they don't already have an explicit value for that attribute. This ensures explicit node attributes always take priority over stylesheet defaults.

## Phase 2: Validate

Runs all lint rules against the graph. Logs warnings, aborts on errors. The engine passes the handler registry's known types to the validation runner so the `type_known` rule can flag unrecognized node types.

Fails on: missing start/exit nodes, incoming edges on start node, outgoing edges on exit nodes, dangling edge references, invalid condition syntax, malformed stylesheets.

## Phase 3: Initialize

1. Generates a UUID run ID (or uses `ResumeRunID`)
2. Creates the run directory (`{logsDir}/{runID}/` with `artifacts/` subdirectory)
3. Creates the event emitter (buffered channel, capacity 256)
4. Creates the artifact store (from config, or auto-created from run directory)
5. Seeds pipeline context from graph `goal` attribute (also sets `graph.goal`) and `InitialContext`
6. Writes initial `manifest.json`
7. If resuming: loads checkpoint, restores context (including logs)/retry counters/completed set

## Phase 4: Execute

The main loop:

```
emit PipelineStarted event
currentNode = startNode (or checkpoint.CurrentNode if resuming)

loop:
    if currentNode is exit:
        check goal gates — if any unsatisfied, resolve retry_target and jump back
        break

    set context["current_node"] = currentNode
    emit StageStarted event
    handler = registry.Lookup(currentNode)
    resolve fidelity mode, generate preamble, set context["internal.preamble"]
    outcome = handler.Execute(...)
    emit StageCompleted or StageFailed event
    write status.json to node log directory
    merge outcome.ContextUpdates into context
    set context["outcome"] = outcome.Status
    set context["preferred_label"] = outcome.PreferredLabel (if non-empty)
    record visit in log

    if RETRY and retries remaining:
        increment counter, set context["internal.retry_count.<id>"]
        emit StageRetrying event
        checkpoint, apply backoff delay (retry_policy), continue
    if RETRY and retries exhausted:
        if allow_partial → PARTIAL_SUCCESS
        else → FAIL

    if FAIL and goal_gate and retries remaining:
        increment counter, checkpoint, apply backoff delay, continue
    if FAIL and goal_gate and no retries → return error (hard failure)

    mark node completed
    nextEdge = edge.Select(graph, currentNode, outcome, context)
    if no matching edge and outcome is failure:
        resolve retry_target chain → jump to target if found
    checkpoint(nextEdge.To)  // emits CheckpointSaved event
    currentNode = nextEdge.To
```

### Retry Behavior

Two retry paths:

1. **Explicit retry**: handler returns `RETRY` status. Engine retries up to `max_retries` (default: graph `default_max_retry`, which defaults to 50). Each retry applies a backoff delay per the node's `retry_policy` (default: `"standard"` — exponential backoff with jitter).
2. **Goal gate**: handler returns `FAIL` on a node with `goal_gate="true"`. Engine retries up to `max_retries` with backoff. If retries exhausted, the entire pipeline fails (goal gates are hard requirements).

When retries are exhausted on an explicit retry:
- If the node has `allow_partial="true"`, the status becomes `PARTIAL_SUCCESS` (treated as success for edge selection)
- Otherwise, the status becomes `FAIL`

### Goal Gate Enforcement at Exit

When the engine reaches an exit node, it checks all `goal_gate="true"` nodes in the visit log. If any gate was never visited or has a non-success latest status, the engine:

1. Looks up the gate node's retry counter vs `max_retries`
2. If retries remain: resolves the retry target chain and redirects execution there
3. If no retries remain: returns an error

### Retry Target Chain

When a node fails with no matching outgoing edge, or a goal gate is unsatisfied at exit, the engine resolves a retry target using this priority chain:

1. Node `retry_target` attribute
2. Node `fallback_retry_target` attribute
3. Graph `retry_target` attribute
4. Graph `fallback_retry_target` attribute

Each candidate is validated against `g.NodeByID()` — only existing nodes are accepted.

### Cancellation

The loop checks `ctx.Done()` on every iteration and during backoff delays. A canceled context stops execution immediately.

## Phase 5: Finalize

1. Saves final checkpoint (with empty `CurrentNode`)
2. Emits `PipelineCompleted` event (or `PipelineFailed` if execute returned error)
3. Closes the event emitter (drains pending events)
4. Updates `manifest.json` with `status: "completed"`

## Run Directory Layout

```
{logsDir}/{runID}/
    manifest.json          # run metadata
    checkpoint.json        # execution state for resumption
    artifacts/             # pipeline-level artifacts (managed by artifact.Store)
        {id}.dat           # file-backed artifacts (≥100KB)
    {node_id}/             # per-node directory (created on demand)
        status.json        # node execution outcome (written after each handler)
        prompt.txt         # codergen prompt (if applicable)
        response.txt       # codergen response (if applicable)
```

## Resumption

To resume a failed/interrupted run:

```go
attractor.RunPipeline(ctx, dotSource, attractor.EngineConfig{
    ResumeRunID: "previous-run-uuid",
    LogsDir:     "runs",
})
```

The engine loads the checkpoint, restores context and retry counters, and resumes from `CurrentNode`. Already-completed nodes are skipped.

## Test Pipelines

DOT fixtures in `testdata/pipelines/` provide integration test coverage for engine features. Each fixture exercises a specific subset of the engine.

### Basic Fixtures

| Fixture | Topology | Features exercised |
|---------|----------|-------------------|
| `simple.dot` | start → exit | Minimal valid pipeline, parse + validate + execute lifecycle |
| `linear.dot` | start → A → B → exit | Edge chaining, codergen handler, checkpoint writes |
| `branching.dot` | start → diamond → {path_a, path_b} → exit | Conditional edge routing, condition expressions |
| `parallel.dot` | start → fan_out → {a, b} → fan_in → exit | Parallel handler, fan-in handler, `wait_all` join |

### Advanced Fixtures

| Fixture | Key features |
|---------|-------------|
| `retry.dot` | `default_max_retry` (graph attr inheritance), `fallback_retry_target`, multiple `retry_policy` presets (`aggressive`, `linear`, `none`), `allow_partial` (PARTIAL_SUCCESS on exhaustion), two `goal_gate` nodes, conditional + labeled edge routing |
| `full_features.dot` | `model_stylesheet` (wildcard, shape/class, ID selectors), `fidelity` (graph-level compact, node-level summary:high, ID-level truncate), parallel with `error_policy="fail_fast"` + `max_parallel="2"`, fan-in, PreferredLabel edge selection (review → fix_issues loop), explicit attr override vs stylesheet defaults |

### Invalid Fixtures (`testdata/pipelines/invalid/`)

| Fixture | Validation error |
|---------|-----------------|
| `no_start.dot` | Missing start node |
| `no_terminal.dot` | Missing exit node |
| `unreachable.dot` | Unreachable nodes |

### Integration Tests

The `engine_test.go` file contains integration tests that use both loaded fixtures and inline DOT graphs. Key integration tests:

| Test | What it verifies |
|------|-----------------|
| `TestDefaultMaxRetryGraphAttr` | Nodes without `max_retries` inherit `default_max_retry` from graph |
| `TestEdgeSelectionSuggestedNextIDs` | Handler `SuggestedNextIDs` drives edge selection (priority step 3) |
| `TestEdgeSelectionPreferredLabel` | Handler `PreferredLabel` drives labeled edge selection (priority step 2) |
| `TestFidelityModeTruncateInEngine` | Truncate preamble contains goal but not stage list |
| `TestEventSequencing` | PipelineStarted → StageStarted → StageCompleted → PipelineCompleted order |
| `TestParallelFanInContextKeys` | Fan-in writes `parallel.fan_in.best_id` / `best_outcome` to context |
| `TestRetryDotFullChain` | Full `retry.dot`: goal gate retry, allow_partial, default_max_retry inheritance |
| `TestFullFeaturesPipeline` | Full `full_features.dot`: stylesheet application + review loop with PreferredLabel |

Tests use custom handler stubs (`perNodeHandler`, `suggestedNextHandler`, `preferredLabelHandler`, etc.) registered via custom registries to isolate engine behavior from real LLM backends.
