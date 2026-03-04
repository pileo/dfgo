# Simulation Backend

**Package**: `internal/attractor/simulate`

A simulation backend for testing Attractor pipelines without live LLM API calls. Provides deterministic, configurable responses for any LLM-backed node type.

## Motivation

Pipelines with `codergen` or `coding_agent` nodes require API keys and make real LLM calls. This blocks CI testing and slows iteration. The simulation backend replaces these handlers with rule-based deterministic responses.

## Config Format

```json
{
  "rules": [
    {"node_id": "review", "response": "LGTM", "context_updates": {"reviewed": "true"}},
    {"node_type": "codergen", "response": "generated code here"},
    {"pattern": "test.*coverage", "response": "95% coverage", "delay": "100ms"},
    {"node_id": "flaky", "status": "retry", "response": "transient failure"},
    {"node_id": "broken", "status": "fail", "error": "service unavailable"}
  ],
  "fallback": "simulated response"
}
```

### Rule Fields

| Field | Description |
|-------|-------------|
| `node_id` | Match by exact node ID |
| `node_type` | Match by node type attribute |
| `pattern` | Match by regex against the node's prompt |
| `response` | Simulated response text |
| `status` | Outcome status: `success` (default), `fail`, `retry` |
| `delay` | Simulated latency (e.g., `100ms`, `2s`) |
| `error` | Error message for fail/retry outcomes |
| `context_updates` | Additional key-value pairs merged into outcome |

### Rule Priority

Rules are matched in priority order: **node ID > node type > prompt regex > fallback**. Within each tier, the first matching rule wins.

## Integration Points

### Backend

`Backend` implements `handler.CodergenBackend` for use as a drop-in replacement in the codergen handler. Matches rules using `opts["node_id"]`, prompt regex, then fallback.

### Handler

`Handler` implements `handler.Handler` and is registered to override `codergen` and `coding_agent` types in the registry, bypassing the entire LLM path. Maps rule status to `runtime.Outcome`.

### BuildRegistry

`BuildRegistry(cfg)` creates a `handler.Registry` with the simulation Handler for `codergen` and `coding_agent`, plus all default handlers for other types (start, exit, conditional, wait.human, etc.).

## CLI Usage

```bash
dfgo run --simulate sim.json pipeline.dot
```

When `--simulate` is set:
- The simulation config is loaded and validated
- A simulation registry replaces the default handler registry
- `AutoApprove` is forced to `true`
- No LLM client is created

The `serve` command also accepts `--simulate` for server-wide simulation mode.

## HTTP Usage

Submit a pipeline with inline simulation config:

```json
POST /api/v1/pipelines
{
  "dot_source": "digraph { ... }",
  "simulate": {
    "rules": [{"node_id": "A", "response": "done"}],
    "fallback": "ok"
  }
}
```

Per-request `simulate` config takes priority over the server-level default.
