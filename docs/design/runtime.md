# Runtime State

**Package**: `internal/attractor/runtime`

Holds all mutable execution state: the pipeline context, stage outcomes, and checkpoint persistence.

## Context

A thread-safe key-value store (`map[string]string` under a `sync.RWMutex`) that accumulates state as the pipeline runs.

```go
ctx := runtime.NewContext()
ctx.Set("project", "dfgo")
val, ok := ctx.Get("project")    // "dfgo", true
ctx.Merge(map[string]string{"a": "1", "b": "2"})
snap := ctx.Snapshot()            // returns a copy
clone := ctx.Clone()              // independent deep copy
ctx.Delete("a")
```

### Thread Safety

All methods are goroutine-safe. `Get` and `Snapshot` use read locks; `Set`, `Delete`, and `Merge` use write locks. This matters for parallel handler execution where multiple goroutines may read/write context concurrently.

### How Context Flows

1. Engine seeds context from graph-level `goal` attribute and `EngineConfig.InitialContext`
2. After each handler executes, `Outcome.ContextUpdates` are merged into context
3. Edge condition evaluation reads context via `context.*` keys
4. Checkpoint saves a snapshot; resume restores it

Convention: handlers store outputs under `{node_id}.response`, `{node_id}.stdout`, etc. to avoid collisions.

## Outcome

The result of executing a single pipeline stage.

```go
type Outcome struct {
    Status          StageStatus        // SUCCESS, FAIL, RETRY, PARTIAL_SUCCESS
    PreferredLabel  string             // hints edge selection toward a labeled edge
    SuggestedNextID string             // hints edge selection toward a specific node
    ContextUpdates  map[string]string  // key-value pairs to merge into pipeline context
    FailureReason   string             // human-readable explanation of failure
    FailureClass    FailureClass       // transient, deterministic, canceled, budget_exhausted
    Notes           string             // handler-specific notes
}
```

### Stage Status

| Status | Meaning | Engine behavior |
|---|---|---|
| `SUCCESS` | Stage completed normally | Advance to next node via edge selection |
| `FAIL` | Stage failed | Check goal gate → retry or abort |
| `RETRY` | Stage requests retry | Retry if under `max_retries`, else fail |
| `PARTIAL_SUCCESS` | Partial completion (treated as success) | Advance to next node |

### Failure Classification

| Class | Meaning | Retry? |
|---|---|---|
| `transient` | Temporary issue (network, rate limit) | Yes |
| `deterministic` | Will always fail (bad prompt, missing config) | No |
| `canceled` | Explicitly canceled by user/system | No |
| `budget_exhausted` | Token/cost budget exceeded | No |

### Convenience Constructors

```go
runtime.SuccessOutcome()
runtime.SuccessOutcomeWithLabel("approved")
runtime.FailOutcome("timeout", runtime.FailureTransient)
runtime.RetryOutcome("rate limited")
```

## Checkpoint

Captures full execution state for crash recovery and resumption.

```go
type Checkpoint struct {
    RunID         string
    CurrentNode   string             // node to resume from
    Completed     []string           // node IDs already finished
    RetryCounters map[string]int     // per-node retry counts
    Context       map[string]string  // full context snapshot
    VisitLog      []VisitEntry       // ordered history of node visits
}
```

### Atomic Writes

Checkpoints are saved atomically: write to `checkpoint.json.tmp`, then `os.Rename` to `checkpoint.json`. If the process crashes mid-write, the previous valid checkpoint remains intact.

```go
cp.Save("/path/to/checkpoint.json")    // atomic write
cp, err := runtime.LoadCheckpoint(path) // read back
```

### When Checkpoints Are Saved

The engine saves a checkpoint:
- Before transitioning to the next node
- Before retrying a node
- At finalization (with `CurrentNode` set to empty)
