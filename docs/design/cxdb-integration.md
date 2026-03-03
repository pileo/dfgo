# CXDB Integration Design

Detailed implementation plan for integrating CXDB into dfgo's pipeline engine.

## Overview

CXDB is an AI context store providing fast, branch-friendly storage for conversation
histories and tool outputs. It stores immutable turns in a DAG structure with
content-addressed blob deduplication (BLAKE3-256).

dfgo uses CXDB for:
- **Event recording** -- every pipeline/agent event becomes a typed CXDB turn
- **Observability** -- browseable execution history in CXDB UI with rich rendering
- **Provenance** -- git repo, branch, process identity captured per context
- **Resume** -- reconstruct execution state from CXDB context head (future)

## Architecture

Each Attractor run maps to **one CXDB context** (one trajectory head). All pipeline
stages and agent events append to this single context linearly. Parallel branch
events are distinguishable via `node_id` and `branch_key` fields in the turn data.

```
Engine.Run()
  │
  ├── Initialize
  │   └── cxdb.Dial(addr)            → client
  │   └── client.CreateContext(0)     → contextID
  │   └── PublishBundle(httpAddr)     → (background goroutine)
  │   └── appendProvenance()          → first turn: ConversationItem with ContextMetadata
  │   └── Events.On(rec.OnEvent)      → subscribe to pipeline events
  │
  ├── Execute (per node)
  │   ├── pipeline event: stage.started    → com.dfgo.stage.started turn
  │   ├── handler.Execute()
  │   │   ├── RecordInput(nodeID, prompt)  → ConversationItem (UserInput)
  │   │   └── agent events:
  │   │       ├── LLMResponse              → ConversationItem (AssistantTurn + TurnMetrics)
  │   │       ├── ToolEnd                  → ConversationItem (AssistantTurn + ToolCallItem)
  │   │       ├── TurnStart                → ConversationItem (SystemMessage info)
  │   │       └── LoopDetected             → ConversationItem (SystemMessage warning)
  │   ├── pipeline event: stage.completed  → com.dfgo.stage.completed turn
  │   └── checkpoint.saved                 → com.dfgo.checkpoint.saved turn
  │
  └── Finalize
      ├── pipeline.completed               → com.dfgo.pipeline.completed turn
      ├── final.json                       → {run_id, cxdb_context_id, ...}
      └── client.Close()
```

## CXDB Connection Details

- **Binary protocol**: `localhost:9009` (Go client, high throughput)
- **HTTP/JSON API**: `localhost:9010` (UI, registry publishing, derived as binary port + 1)
- **Go client module**: `github.com/strongdm/ai-cxdb/clients/go`

The recorder strips `http://` and `https://` scheme prefixes from the address, since
`cxdb.Dial` expects a raw `host:port` for TCP.

## Package: `internal/attractor/cxdbstore`

```
internal/attractor/cxdbstore/
├── recorder.go      # Recorder struct, lifecycle, event callbacks, provenance, git capture
├── turns.go         # Turn type structs, event-to-turn mapping, ConversationItem encoding
├── registry.go      # Registry bundle definition + HTTP publish
└── recorder_test.go # Tests with mock CXDBClient
```

### CXDBClient Interface

The recorder depends on an interface rather than a concrete client, enabling
mock-based testing without a running CXDB server:

```go
type CXDBClient interface {
    CreateContext(ctx context.Context, baseTurnID uint64) (*cxdb.ContextHead, error)
    ForkContext(ctx context.Context, baseTurnID uint64) (*cxdb.ContextHead, error)
    AppendTurn(ctx context.Context, req *cxdb.AppendRequest) (*cxdb.AppendResult, error)
    Close() error
}
```

### Recorder

The central integration point. Thread-safe via `sync.Mutex`.

```go
type Config struct {
    Address   string // "localhost:9009" (scheme stripped automatically)
    ClientTag string // "dfgo"
    RunID     string
    Pipeline  string
}

type Recorder struct {
    client    CXDBClient
    contextID uint64
    headTurn  uint64
    runID     string
    pipeline  string
    mu        sync.Mutex
}
```

Key methods:
- `New(ctx, cfg)` -- dials CXDB, creates context, publishes bundle (background), appends provenance
- `NewWithClient(ctx, client, runID, pipeline)` -- for testing with mock clients
- `OnEvent(evt)` -- implements `events.Callback` for pipeline events
- `OnAgentEvent(nodeID, evt)` -- records agent events as ConversationItem v3 turns
- `RecordInput(nodeID, text)` -- records the prompt sent to an agent as a UserInput turn
- `Fork(ctx)` -- creates a new context branching from current head (retained for future use)
- `Close()` -- closes the underlying CXDB client

### Provenance

The first turn of every context is a `cxdb.ConversationItem` (v3) containing:
- `ContextMetadata` with client tag, title, labels, custom fields
- `Provenance` with process identity, git repo/branch, correlation ID

Git information is captured best-effort by shelling out to `git`:
- `git remote get-url origin` → parsed to `org/repo` slug (handles SSH and HTTPS URLs)
- `git rev-parse --abbrev-ref HEAD` → branch name

### Turn Types

Pipeline events use custom `com.dfgo.*` types with msgpack numeric field tags.
Agent events use canonical `cxdb.ConversationItem` v3 types for rich CXDB UI rendering.

#### Pipeline Event Types (custom, with registry)

| Type ID | Fields |
|---------|--------|
| `com.dfgo.pipeline.started` | run_id, pipeline, start_node, timestamp |
| `com.dfgo.pipeline.completed` | run_id, pipeline, status, timestamp |
| `com.dfgo.pipeline.failed` | run_id, error, timestamp |
| `com.dfgo.stage.started` | node_id, node_type, shape, timestamp |
| `com.dfgo.stage.completed` | node_id, status, notes, timestamp |
| `com.dfgo.stage.failed` | node_id, status, failure_reason, failure_class, timestamp |
| `com.dfgo.stage.retrying` | node_id, attempt, max_retry, timestamp |
| `com.dfgo.checkpoint.saved` | current_node, commit_sha, timestamp |
| `com.dfgo.parallel.started` | node_id, branch_count, branch_ids, join_policy, timestamp |
| `com.dfgo.parallel.branch` | node_id, branch_key, event, status, timestamp |
| `com.dfgo.parallel.completed` | node_id, winner_key, join_policy, timestamp |
| `com.dfgo.interview.started` | node_id, event, question, answer, timestamp |
| `com.dfgo.interview.completed` | node_id, event, question, answer, timestamp |
| `com.dfgo.interview.timeout` | node_id, event, question, answer, timestamp |

#### Agent Event Types (ConversationItem v3)

Agent events are recorded as canonical `cxdb.ConversationItem` v3 turns so the CXDB UI
renders them with rich tool call, metrics, and system message widgets.

| Agent Event | ConversationItem Mapping |
|-------------|-------------------------|
| `turn.start` | `ItemTypeSystem` / `SystemKindInfo` -- "Agent turn N" |
| `llm.response` | `ItemTypeAssistantTurn` with `AssistantTurn.Text`, `FinishReason`, `TurnMetrics` (model, input/output/total tokens) |
| `tool.end` | `ItemTypeAssistantTurn` with `ToolCallItem` (ID, name, args, status, result content) |
| `loop.detected` | `ItemTypeSystem` / `SystemKindWarning` -- tool name and stage |

Additionally, `RecordInput()` records the prompt sent to the agent as:
- `ItemTypeUserInput` with `UserInput.Text`

Tool call items include:
- `ID` -- tool call identifier from the LLM
- `Name` -- tool name (read, write, shell, etc.)
- `Args` -- JSON-encoded tool arguments (e.g., `{"path":"/tmp/foo.txt"}`)
- `Status` -- `complete` or `error`
- `Result.Content` -- tool output (truncated to 4KB)
- `Result.Success` -- boolean

LLM response turns include:
- `Text` -- assistant response text (truncated to 4KB)
- `FinishReason` -- why generation stopped
- `Metrics.Model` -- model name
- `Metrics.InputTokens`, `OutputTokens`, `TotalTokens`

### Registry Bundle

Published to CXDB's HTTP API on startup (best-effort, background goroutine) so
pipeline turn types render with field names in the CXDB UI.

- **Bundle ID**: `com.attractor.dfgo-v1`
- **HTTP endpoint**: `PUT /v1/registry/bundles/{bundle_id}`
- **HTTP address**: derived from binary port + 1 (e.g., 9009 → `http://localhost:9010`)
- **Format**: CXDB versioned type descriptor with `"versions"` wrapping and `"semantic"` hints

Agent events use the canonical `cxdb.ConversationItem` type which has its own
registry bundle (`cxdb.conversation-item-v3`) already published by the CXDB server.

## Engine Integration

### EngineConfig

```go
type EngineConfig struct {
    // ... existing fields ...
    CXDBAddr string // CXDB binary protocol address; empty = disabled
}
```

### Initialize (Phase 3)

After event emitter creation, if `CXDBAddr` is set:
1. Create recorder via `cxdbstore.New(ctx, cfg)` -- fails fast if unreachable
2. Subscribe to pipeline events via `Events.On(rec.OnEvent)`
3. Provenance appended as first turn (with git repo/branch)
4. Registry bundle published in background goroutine

### Execute (Phase 4)

In `executeNode()`, handlers are wired at runtime (same pattern as `ChildExecutor`):
- `CodingAgentHandler.SetRecorder(e.Recorder)` -- forwards agent events
- Before `session.Run()`, calls `recorder.RecordInput(nodeID, prompt)` to log the input

### Finalize (Phase 5)

1. Save final checkpoint (with CXDB context ID and head turn)
2. Emit `pipeline.completed` event
3. Write `final.json` with `cxdb_context_id`
4. Close recorder

### Checkpoint Fields

```go
type Checkpoint struct {
    // ... existing fields ...
    CXDBContextID uint64 `json:"cxdb_context_id,omitempty"`
    CXDBHeadTurn  uint64 `json:"cxdb_head_turn,omitempty"`
}
```

## CLI

```
dfgo run pipeline.dot --cxdb localhost:9009
```

Also reads from environment: `DFGO_CXDB_ADDR`. Flag takes priority.

## One Context Per Run

Per the [kilroy metaspec](https://github.com/danshapiro/kilroy/blob/main/docs/strongdm/attractor/kilroy-metaspec.md),
each Attractor run maps to one CXDB context. Parallel branches do **not** create
forked contexts. All events (pipeline stages, agent turns, tool calls) append
linearly to the single root context. Branch events carry `node_id` and `branch_key`
fields for disambiguation.

The `Fork()` method is retained on the Recorder for potential future use (e.g.,
resume from checkpoint creating a new branch), but is not called during normal
pipeline execution.

## Dependencies

```
require (
    github.com/strongdm/ai-cxdb/clients/go v0.0.0
)

replace github.com/strongdm/ai-cxdb/clients/go => ../cxdb/clients/go
```

Transitive: `vmihailenco/msgpack/v5`, `zeebo/blake3`, `klauspost/cpuid`.

## Testing

### Unit Tests (recorder_test.go)

All tests use a mock `CXDBClient` implementation. Coverage includes:
- Recorder creation and context ID assignment
- Provenance turn encoding (ContextMetadata, Provenance fields)
- All 15 pipeline event types mapped to correct turn types
- Agent events mapped to ConversationItem v3 with decoded verification:
  - TurnStart → SystemMessage info with turn number
  - LLMResponse → AssistantTurn with text, model, metrics, finish reason
  - ToolEnd → AssistantTurn with ToolCallItem (ID, name, args, result, status)
  - ToolEnd with error → ToolCallStatusError
  - LoopDetected → SystemMessage warning
- Unknown events silently ignored
- Fork creates new recorder with different context ID
- Encode/decode round-trip for pipeline structs and ConversationItem types
- Head turn advances on each append
- Registry bundle format validation (versioned structure, all types present)
- HTTPAddrFromBinary port derivation
- Helper functions (str, intVal, boolVal)

### Manual Verification

Run pipeline, then browse `http://localhost:9010` to see turns in CXDB UI.
Agent tool calls show with arguments and results; LLM responses show with
token metrics and model name.
