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
2. Engine sets built-in keys at various points (see below)
3. After each handler executes, `Outcome.ContextUpdates` are merged into context
4. Edge condition evaluation reads context via `context.*` keys
5. Checkpoint saves a snapshot; resume restores it

Convention: handlers store outputs under `{node_id}.response`, `{node_id}.stdout`, etc. to avoid collisions.

### Built-in Context Keys

The engine automatically sets these keys:

| Key | When set | Value |
|-----|----------|-------|
| `goal` | Initialization | Graph-level `goal` attribute |
| `graph.goal` | Initialization | Graph-level `goal` attribute (alias) |
| `current_node` | Before each handler call | Current node ID |
| `outcome` | After each handler call | Status string (e.g., `"SUCCESS"`, `"FAIL"`) |
| `preferred_label` | After each handler call | Outcome's preferred label (if non-empty) |
| `internal.retry_count.<nodeID>` | On retry | Retry count as string (e.g., `"1"`, `"2"`) |

## Outcome

The result of executing a single pipeline stage.

```go
type Outcome struct {
    Status           StageStatus        // SUCCESS, FAIL, RETRY, PARTIAL_SUCCESS
    PreferredLabel   string             // hints edge selection toward a labeled edge
    SuggestedNextIDs []string           // hints edge selection toward specific nodes (priority order)
    ContextUpdates   map[string]string  // key-value pairs to merge into pipeline context
    FailureReason    string             // human-readable explanation of failure
    FailureClass     FailureClass       // transient, deterministic, canceled, budget_exhausted
    Notes            string             // handler-specific notes
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

## Retry Policy

Configures exponential backoff delay between retry attempts.

```go
type RetryPolicy struct {
    InitialDelayMs int
    BackoffFactor  float64
    MaxDelayMs     int
    Jitter         bool     // when true, delays are randomized to 50-100% of computed value
}
```

`DelayForAttempt(n)` computes: `initial * factor^(n-1)`, capped at `MaxDelayMs`, with optional jitter.

### Presets

Select a preset via the `retry_policy` node attribute (default: `"standard"`):

| Name | Initial | Factor | Max | Jitter | Use case |
|------|---------|--------|-----|--------|----------|
| `none` | 0ms | 1.0 | 0 | no | Tests, instant retry |
| `standard` | 200ms | 2.0 | 60s | yes | Default for most nodes |
| `aggressive` | 50ms | 1.5 | 5s | yes | Fast retry with cap |
| `linear` | 1000ms | 1.0 | 1s | no | Fixed 1s delay |
| `patient` | 1000ms | 3.0 | 120s | yes | Slow external services |

```go
policy := runtime.PolicyByName("standard")  // falls back to "standard" for unknown names
delay := policy.DelayForAttempt(3)           // ~800ms ± jitter
```
