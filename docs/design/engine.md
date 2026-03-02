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
    Interviewer     interviewer.Interviewer
    AutoApprove     bool                  // use AutoApprove interviewer
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

## Phase 2: Validate

Runs all lint rules against the graph. Logs warnings, aborts on errors.

Fails on: missing start/exit nodes, dangling edge references, invalid condition syntax.

## Phase 3: Initialize

1. Generates a UUID run ID (or uses `ResumeRunID`)
2. Creates the run directory (`{logsDir}/{runID}/` with `artifacts/` subdirectory)
3. Seeds pipeline context from graph `goal` attribute and `InitialContext`
4. Writes initial `manifest.json`
5. If resuming: loads checkpoint, restores context/retry counters/completed set

## Phase 4: Execute

The main loop:

```
currentNode = startNode (or checkpoint.CurrentNode if resuming)

loop:
    if currentNode is exit → break
    handler = registry.Lookup(currentNode)
    outcome = handler.Execute(...)
    merge outcome.ContextUpdates into context
    record visit in log

    if RETRY and retries remaining → increment counter, checkpoint, continue
    if FAIL and goal_gate and retries remaining → increment counter, checkpoint, continue
    if FAIL and goal_gate and no retries → return error (hard failure)

    mark node completed
    nextEdge = edge.Select(graph, currentNode, outcome, context)
    checkpoint(nextEdge.To)
    currentNode = nextEdge.To
```

### Retry Behavior

Two retry paths:

1. **Explicit retry**: handler returns `RETRY` status. Engine retries up to `max_retries`.
2. **Goal gate**: handler returns `FAIL` on a node with `goal_gate="true"`. Engine retries up to `max_retries`. If retries exhausted, the entire pipeline fails (goal gates are hard requirements).

### Cancellation

The loop checks `ctx.Done()` on every iteration. A canceled context stops execution immediately.

## Phase 5: Finalize

1. Saves final checkpoint (with empty `CurrentNode`)
2. Updates `manifest.json` with `status: "completed"`

## Run Directory Layout

```
{logsDir}/{runID}/
    manifest.json          # run metadata
    checkpoint.json        # execution state for resumption
    artifacts/             # pipeline-level artifacts
    {node_id}/             # per-node directory (created on demand)
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
