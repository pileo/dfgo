# CXDB Integration Design

Detailed implementation plan for integrating CXDB into dfgo's pipeline engine.

## Overview

CXDB is an AI context store providing fast, branch-friendly storage for conversation
histories and tool outputs. It stores immutable turns in a DAG structure with
content-addressed blob deduplication (BLAKE3-256).

dfgo will use CXDB for:
- **Event recording** -- every pipeline/agent event becomes a typed CXDB turn
- **Artifact storage** -- large payloads stored as CXDB blobs
- **Parallel branch tracking** -- context forks for fan-out/fan-in
- **Resume** -- reconstruct execution state from CXDB context head
- **Observability** -- browseable execution history in CXDB UI

## Architecture

```
Engine.Run()
  │
  ├── Initialize
  │   └── cxdb.Dial("localhost:9009") → client
  │   └── client.CreateContext(0)     → contextID
  │   └── appendTurn(pipeline.started)
  │
  ├── Execute (per node)
  │   ├── appendTurn(stage.started)
  │   ├── handler.Execute()
  │   │   └── [agent events → appendTurn(agent.*)]
  │   ├── appendTurn(stage.completed | stage.failed)
  │   └── appendTurn(checkpoint.saved)
  │
  ├── Parallel branches
  │   ├── client.ForkContext(headTurnID) → branchContextID
  │   ├── branch events → appendTurn on branchContextID
  │   └── fan-in → appendTurn(parallel.completed) on main context
  │
  └── Finalize
      ├── appendTurn(pipeline.completed | pipeline.failed)
      └── client.Close()
```

## CXDB Connection Details

- **Binary protocol**: `localhost:9009` (Go client, high throughput)
- **HTTP/JSON API**: `localhost:9010` (UI, tooling)
- **Go client**: `github.com/strongdm/cxdb/clients/go`

```go
import cxdb "github.com/strongdm/cxdb/clients/go"

client, err := cxdb.Dial("localhost:9009",
    cxdb.WithDialTimeout(5*time.Second),
    cxdb.WithRequestTimeout(30*time.Second),
    cxdb.WithClientTag("dfgo-"+runID),
)
```

## New Package: `internal/attractor/cxdbstore`

Single package containing the recorder, turn types, and registry bundle.

### Files

```
internal/attractor/cxdbstore/
├── recorder.go      # Recorder struct, lifecycle, event callback
├── turns.go         # Turn type structs with msgpack tags
├── registry.go      # Registry bundle definition + publish
└── recorder_test.go # Tests with mock client
```

### Recorder

The central integration point. Implements the `events.Callback` signature and
appends turns to CXDB in response to pipeline events.

```go
// recorder.go

type Config struct {
    Address    string // "localhost:9009"
    ClientTag  string // "dfgo"
    RunID      string
    Pipeline   string
}

type Recorder struct {
    client    *cxdb.Client
    contextID uint64
    headTurn  uint64
    runID     string
    pipeline  string
    mu        sync.Mutex
}

func New(ctx context.Context, cfg Config) (*Recorder, error) {
    client, err := cxdb.Dial(cfg.Address,
        cxdb.WithClientTag(cfg.ClientTag),
    )
    if err != nil {
        return nil, fmt.Errorf("cxdb dial: %w", err)
    }

    head, err := client.CreateContext(ctx, 0)
    if err != nil {
        client.Close()
        return nil, fmt.Errorf("cxdb create context: %w", err)
    }

    return &Recorder{
        client:    client,
        contextID: head.ContextID,
        headTurn:  head.HeadTurnID,
        runID:     cfg.RunID,
        pipeline:  cfg.Pipeline,
    }, nil
}

func (r *Recorder) ContextID() uint64 { return r.contextID }

// OnEvent implements events.Callback for pipeline events.
func (r *Recorder) OnEvent(evt events.Event) {
    turn, ok := eventToTurn(evt)
    if !ok {
        return
    }
    r.append(turn.typeID, turn.typeVersion, turn.payload)
}

// OnAgentEvent records agent-level events within a stage.
func (r *Recorder) OnAgentEvent(nodeID string, evt event.Event) {
    turn, ok := agentEventToTurn(nodeID, evt)
    if !ok {
        return
    }
    r.append(turn.typeID, turn.typeVersion, turn.payload)
}

// Fork creates a new CXDB context branching from the current head.
// Used for parallel branches.
func (r *Recorder) Fork(ctx context.Context) (*Recorder, error) {
    r.mu.Lock()
    headTurn := r.headTurn
    r.mu.Unlock()

    fork, err := r.client.ForkContext(ctx, headTurn)
    if err != nil {
        return nil, fmt.Errorf("cxdb fork: %w", err)
    }

    return &Recorder{
        client:    r.client, // shared client
        contextID: fork.ContextID,
        headTurn:  fork.HeadTurnID,
        runID:     r.runID,
        pipeline:  r.pipeline,
    }, nil
}

func (r *Recorder) Close() error {
    return r.client.Close()
}

// append is the internal write path.
func (r *Recorder) append(typeID string, typeVersion uint32, payload []byte) {
    r.mu.Lock()
    defer r.mu.Unlock()

    result, err := r.client.AppendTurn(context.Background(), &cxdb.AppendRequest{
        ContextID:   r.contextID,
        TypeID:      typeID,
        TypeVersion: typeVersion,
        Payload:     payload,
    })
    if err != nil {
        slog.Error("cxdb append failed", "type", typeID, "error", err)
        return
    }
    r.headTurn = result.TurnID
}
```

### Turn Types

Each pipeline event maps to a typed CXDB turn. Payloads use msgpack with numeric
field tags for schema evolution.

```go
// turns.go

const (
    TypePipelineStarted  = "com.dfgo.pipeline.started"
    TypePipelineCompleted = "com.dfgo.pipeline.completed"
    TypePipelineFailed   = "com.dfgo.pipeline.failed"
    TypeStageStarted     = "com.dfgo.stage.started"
    TypeStageCompleted   = "com.dfgo.stage.completed"
    TypeStageFailed      = "com.dfgo.stage.failed"
    TypeStageRetrying    = "com.dfgo.stage.retrying"
    TypeParallelStarted  = "com.dfgo.parallel.started"
    TypeParallelBranch   = "com.dfgo.parallel.branch"
    TypeParallelCompleted = "com.dfgo.parallel.completed"
    TypeInterviewStarted  = "com.dfgo.interview.started"
    TypeInterviewCompleted = "com.dfgo.interview.completed"
    TypeInterviewTimeout  = "com.dfgo.interview.timeout"
    TypeCheckpointSaved  = "com.dfgo.checkpoint.saved"
    TypeAgentTurnStart   = "com.dfgo.agent.turn.start"
    TypeAgentLLMResponse = "com.dfgo.agent.llm.response"
    TypeAgentToolExec    = "com.dfgo.agent.tool.exec"
    TypeAgentLoopDetected = "com.dfgo.agent.loop.detected"
)

// All turn structs use msgpack numeric tags.
// Tags are never reused within a type (append-only schema evolution).

type PipelineStartedTurn struct {
    RunID    string `msgpack:"1"`
    Pipeline string `msgpack:"2"`
    StartNode string `msgpack:"3"`
    Timestamp uint64 `msgpack:"4"` // unix_ms
}

type PipelineCompletedTurn struct {
    RunID    string `msgpack:"1"`
    Pipeline string `msgpack:"2"`
    Status   string `msgpack:"3"` // "completed"
    Timestamp uint64 `msgpack:"4"`
}

type PipelineFailedTurn struct {
    RunID    string `msgpack:"1"`
    Error    string `msgpack:"2"`
    Timestamp uint64 `msgpack:"3"`
}

type StageStartedTurn struct {
    NodeID   string `msgpack:"1"`
    NodeType string `msgpack:"2"`
    Shape    string `msgpack:"3"`
    Timestamp uint64 `msgpack:"4"`
}

type StageCompletedTurn struct {
    NodeID   string `msgpack:"1"`
    Status   string `msgpack:"2"` // success, partial_success, skipped
    Notes    string `msgpack:"3"`
    Timestamp uint64 `msgpack:"4"`
}

type StageFailedTurn struct {
    NodeID       string `msgpack:"1"`
    Status       string `msgpack:"2"` // fail
    FailureReason string `msgpack:"3"`
    FailureClass string `msgpack:"4"` // transient, deterministic, etc.
    Timestamp    uint64 `msgpack:"5"`
}

type StageRetryingTurn struct {
    NodeID   string `msgpack:"1"`
    Attempt  int    `msgpack:"2"`
    MaxRetry int    `msgpack:"3"`
    Timestamp uint64 `msgpack:"4"`
}

type CheckpointSavedTurn struct {
    CurrentNode string `msgpack:"1"`
    CommitSHA   string `msgpack:"2"` // future: git commit SHA
    Timestamp   uint64 `msgpack:"3"`
}

type ParallelStartedTurn struct {
    NodeID      string   `msgpack:"1"`
    BranchCount int      `msgpack:"2"`
    BranchIDs   []string `msgpack:"3"`
    JoinPolicy  string   `msgpack:"4"`
    Timestamp   uint64   `msgpack:"5"`
}

type ParallelBranchTurn struct {
    NodeID    string `msgpack:"1"`
    BranchKey string `msgpack:"2"`
    Event     string `msgpack:"3"` // "started" or "completed"
    Status    string `msgpack:"4"` // only for completed
    Timestamp uint64 `msgpack:"5"`
}

type ParallelCompletedTurn struct {
    NodeID      string `msgpack:"1"`
    WinnerKey   string `msgpack:"2"`
    JoinPolicy  string `msgpack:"3"`
    Timestamp   uint64 `msgpack:"4"`
}

type InterviewTurn struct {
    NodeID    string `msgpack:"1"`
    Event     string `msgpack:"2"` // "started", "completed", "timeout"
    Question  string `msgpack:"3"`
    Answer    string `msgpack:"4"` // only for completed
    Timestamp uint64 `msgpack:"5"`
}

type AgentTurnStartTurn struct {
    NodeID    string `msgpack:"1"`
    Round     int    `msgpack:"2"`
    Timestamp uint64 `msgpack:"3"`
}

type AgentLLMResponseTurn struct {
    NodeID       string `msgpack:"1"`
    Model        string `msgpack:"2"`
    FinishReason string `msgpack:"3"`
    InputTokens  int    `msgpack:"4"`
    OutputTokens int    `msgpack:"5"`
    Timestamp    uint64 `msgpack:"6"`
}

type AgentToolExecTurn struct {
    NodeID   string `msgpack:"1"`
    ToolName string `msgpack:"2"`
    CallID   string `msgpack:"3"`
    IsError  bool   `msgpack:"4"`
    Duration uint64 `msgpack:"5"` // ms
    Timestamp uint64 `msgpack:"6"`
}

type AgentLoopDetectedTurn struct {
    NodeID   string `msgpack:"1"`
    ToolName string `msgpack:"2"`
    Timestamp uint64 `msgpack:"3"`
}
```

### Event-to-Turn Mapping

```go
// turns.go

type turnData struct {
    typeID      string
    typeVersion uint32
    payload     []byte
}

func eventToTurn(evt events.Event) (turnData, bool) {
    ts := uint64(evt.Timestamp.UnixMilli())

    switch evt.Type {
    case events.PipelineStarted:
        return encode(TypePipelineStarted, 1, PipelineStartedTurn{
            RunID:     str(evt.Data, "run_id"),
            Pipeline:  str(evt.Data, "pipeline"),
            StartNode: str(evt.Data, "start"),
            Timestamp: ts,
        })
    case events.PipelineCompleted:
        return encode(TypePipelineCompleted, 1, PipelineCompletedTurn{
            RunID:     str(evt.Data, "run_id"),
            Pipeline:  str(evt.Data, "pipeline"),
            Status:    "completed",
            Timestamp: ts,
        })
    case events.PipelineFailed:
        return encode(TypePipelineFailed, 1, PipelineFailedTurn{
            RunID:     str(evt.Data, "run_id"),
            Error:     str(evt.Data, "error"),
            Timestamp: ts,
        })
    case events.StageStarted:
        return encode(TypeStageStarted, 1, StageStartedTurn{
            NodeID:   str(evt.Data, "node_id"),
            NodeType: str(evt.Data, "type"),
            Shape:    str(evt.Data, "shape"),
            Timestamp: ts,
        })
    case events.StageCompleted:
        return encode(TypeStageCompleted, 1, StageCompletedTurn{
            NodeID:   str(evt.Data, "node_id"),
            Status:   str(evt.Data, "status"),
            Timestamp: ts,
        })
    case events.StageFailed:
        return encode(TypeStageFailed, 1, StageFailedTurn{
            NodeID:       str(evt.Data, "node_id"),
            Status:       str(evt.Data, "status"),
            FailureReason: str(evt.Data, "reason"),
            FailureClass: str(evt.Data, "failure_class"),
            Timestamp:    ts,
        })
    case events.StageRetrying:
        return encode(TypeStageRetrying, 1, StageRetryingTurn{
            NodeID:   str(evt.Data, "node_id"),
            Attempt:  intVal(evt.Data, "attempt"),
            MaxRetry: intVal(evt.Data, "max"),
            Timestamp: ts,
        })
    case events.CheckpointSaved:
        return encode(TypeCheckpointSaved, 1, CheckpointSavedTurn{
            CurrentNode: str(evt.Data, "current_node"),
            Timestamp:   ts,
        })
    case events.ParallelStarted:
        return encode(TypeParallelStarted, 1, ParallelStartedTurn{
            NodeID:      str(evt.Data, "node_id"),
            BranchCount: intVal(evt.Data, "branch_count"),
            JoinPolicy:  str(evt.Data, "join_policy"),
            Timestamp:   ts,
        })
    case events.ParallelBranchStarted:
        return encode(TypeParallelBranch, 1, ParallelBranchTurn{
            NodeID:    str(evt.Data, "node_id"),
            BranchKey: str(evt.Data, "branch_key"),
            Event:     "started",
            Timestamp: ts,
        })
    case events.ParallelBranchCompleted:
        return encode(TypeParallelBranch, 1, ParallelBranchTurn{
            NodeID:    str(evt.Data, "node_id"),
            BranchKey: str(evt.Data, "branch_key"),
            Event:     "completed",
            Status:    str(evt.Data, "status"),
            Timestamp: ts,
        })
    case events.ParallelCompleted:
        return encode(TypeParallelCompleted, 1, ParallelCompletedTurn{
            NodeID:     str(evt.Data, "node_id"),
            WinnerKey:  str(evt.Data, "winner"),
            JoinPolicy: str(evt.Data, "join_policy"),
            Timestamp:  ts,
        })
    case events.InterviewStarted:
        return encode(TypeInterviewStarted, 1, InterviewTurn{
            NodeID:    str(evt.Data, "node_id"),
            Event:     "started",
            Question:  str(evt.Data, "question"),
            Timestamp: ts,
        })
    case events.InterviewCompleted:
        return encode(TypeInterviewCompleted, 1, InterviewTurn{
            NodeID:    str(evt.Data, "node_id"),
            Event:     "completed",
            Answer:    str(evt.Data, "answer"),
            Timestamp: ts,
        })
    case events.InterviewTimeout:
        return encode(TypeInterviewTimeout, 1, InterviewTurn{
            NodeID:    str(evt.Data, "node_id"),
            Event:     "timeout",
            Timestamp: ts,
        })
    default:
        return turnData{}, false
    }
}

func agentEventToTurn(nodeID string, evt event.Event) (turnData, bool) {
    ts := uint64(evt.Timestamp.UnixMilli())

    switch evt.Type {
    case event.TurnStart:
        return encode(TypeAgentTurnStart, 1, AgentTurnStartTurn{
            NodeID:    nodeID,
            Round:     intVal(evt.Data, "round"),
            Timestamp: ts,
        })
    case event.LLMResponse:
        return encode(TypeAgentLLMResponse, 1, AgentLLMResponseTurn{
            NodeID:       nodeID,
            Model:        str(evt.Data, "model"),
            FinishReason: str(evt.Data, "finish_reason"),
            InputTokens:  intVal(evt.Data, "input_tokens"),
            OutputTokens: intVal(evt.Data, "output_tokens"),
            Timestamp:    ts,
        })
    case event.ToolEnd:
        return encode(TypeAgentToolExec, 1, AgentToolExecTurn{
            NodeID:   nodeID,
            ToolName: str(evt.Data, "tool"),
            CallID:   str(evt.Data, "call_id"),
            IsError:  boolVal(evt.Data, "is_error"),
            Timestamp: ts,
        })
    case event.LoopDetected:
        return encode(TypeAgentLoopDetected, 1, AgentLoopDetectedTurn{
            NodeID:   nodeID,
            ToolName: str(evt.Data, "tool"),
            Timestamp: ts,
        })
    default:
        return turnData{}, false
    }
}

func encode(typeID string, version uint32, v any) (turnData, bool) {
    payload, err := cxdb.EncodeMsgpack(v)
    if err != nil {
        slog.Error("cxdb encode failed", "type", typeID, "error", err)
        return turnData{}, false
    }
    return turnData{typeID: typeID, typeVersion: version, payload: payload}, true
}
```

### Registry Bundle

Published once to CXDB so turns render correctly in the UI.

```go
// registry.go

const BundleID = "dfgo-v1"

func RegistryBundle() map[string]any {
    return map[string]any{
        "registry_version": 1,
        "bundle_id":        BundleID,
        "types": map[string]any{
            TypePipelineStarted: typeDesc(1, map[string]any{
                "1": field("run_id", "string"),
                "2": field("pipeline", "string"),
                "3": field("start_node", "string"),
                "4": field("timestamp", "u64", "unix_ms"),
            }),
            TypePipelineCompleted: typeDesc(1, map[string]any{
                "1": field("run_id", "string"),
                "2": field("pipeline", "string"),
                "3": field("status", "string"),
                "4": field("timestamp", "u64", "unix_ms"),
            }),
            TypePipelineFailed: typeDesc(1, map[string]any{
                "1": field("run_id", "string"),
                "2": field("error", "string"),
                "3": field("timestamp", "u64", "unix_ms"),
            }),
            TypeStageStarted: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("node_type", "string"),
                "3": field("shape", "string"),
                "4": field("timestamp", "u64", "unix_ms"),
            }),
            TypeStageCompleted: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("status", "string"),
                "3": field("notes", "string"),
                "4": field("timestamp", "u64", "unix_ms"),
            }),
            TypeStageFailed: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("status", "string"),
                "3": field("failure_reason", "string"),
                "4": field("failure_class", "string"),
                "5": field("timestamp", "u64", "unix_ms"),
            }),
            TypeStageRetrying: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("attempt", "i32"),
                "3": field("max_retry", "i32"),
                "4": field("timestamp", "u64", "unix_ms"),
            }),
            TypeCheckpointSaved: typeDesc(1, map[string]any{
                "1": field("current_node", "string"),
                "2": field("commit_sha", "string"),
                "3": field("timestamp", "u64", "unix_ms"),
            }),
            TypeParallelStarted: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("branch_count", "i32"),
                "3": fieldArray("branch_ids", "string"),
                "4": field("join_policy", "string"),
                "5": field("timestamp", "u64", "unix_ms"),
            }),
            TypeParallelBranch: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("branch_key", "string"),
                "3": field("event", "string"),
                "4": field("status", "string"),
                "5": field("timestamp", "u64", "unix_ms"),
            }),
            TypeParallelCompleted: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("winner_key", "string"),
                "3": field("join_policy", "string"),
                "4": field("timestamp", "u64", "unix_ms"),
            }),
            TypeInterviewStarted: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("event", "string"),
                "3": field("question", "string"),
                "4": field("answer", "string"),
                "5": field("timestamp", "u64", "unix_ms"),
            }),
            TypeInterviewCompleted: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("event", "string"),
                "3": field("question", "string"),
                "4": field("answer", "string"),
                "5": field("timestamp", "u64", "unix_ms"),
            }),
            TypeInterviewTimeout: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("event", "string"),
                "3": field("question", "string"),
                "4": field("answer", "string"),
                "5": field("timestamp", "u64", "unix_ms"),
            }),
            TypeAgentTurnStart: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("round", "i32"),
                "3": field("timestamp", "u64", "unix_ms"),
            }),
            TypeAgentLLMResponse: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("model", "string"),
                "3": field("finish_reason", "string"),
                "4": field("input_tokens", "i32"),
                "5": field("output_tokens", "i32"),
                "6": field("timestamp", "u64", "unix_ms"),
            }),
            TypeAgentToolExec: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("tool_name", "string"),
                "3": field("call_id", "string"),
                "4": field("is_error", "bool"),
                "5": field("duration_ms", "u64", "duration_ms"),
                "6": field("timestamp", "u64", "unix_ms"),
            }),
            TypeAgentLoopDetected: typeDesc(1, map[string]any{
                "1": field("node_id", "string"),
                "2": field("tool_name", "string"),
                "3": field("timestamp", "u64", "unix_ms"),
            }),
        },
    }
}
```

## Engine Integration

### EngineConfig Changes

```go
// attractor.go

type EngineConfig struct {
    // ... existing fields ...

    CXDBAddr string // CXDB binary protocol address (e.g., "localhost:9009")
                    // Empty string = CXDB disabled
}
```

### Engine Changes

Integration points in `engine.go`:

#### Phase 3: Initialize (after event emitter creation)

```go
// engine.go initialize()

// After: e.Events = events.NewEmitter(256)
if e.Config.CXDBAddr != "" {
    rec, err := cxdbstore.New(ctx, cxdbstore.Config{
        Address:   e.Config.CXDBAddr,
        ClientTag: "dfgo",
        RunID:     e.RunID,
        Pipeline:  g.Name,
    })
    if err != nil {
        return fmt.Errorf("cxdb init: %w", err)
    }
    e.Recorder = rec

    // Subscribe to pipeline events
    e.Events.On(rec.OnEvent)
}
```

#### Phase 4: Execute (agent event forwarding)

In `CodingAgentHandler.Execute()`, forward agent events to the recorder:

```go
// handler/coding_agent.go

session.OnEvent(func(evt event.Event) {
    // Existing logging...

    // Forward to CXDB recorder if available
    if h.recorder != nil {
        h.recorder.OnAgentEvent(node.ID, evt)
    }
})
```

The recorder needs to be injected into handlers that emit agent events.
Add a `WithCXDBRecorder` registry option:

```go
// handler/handler.go

func WithCXDBRecorder(r *cxdbstore.Recorder) RegistryOption {
    return func(c *registryConfig) { c.recorder = r }
}
```

#### Parallel Handler: Fork

When the parallel handler creates branches, fork the CXDB context:

```go
// handler/parallel.go

// For each branch:
var branchRecorder *cxdbstore.Recorder
if h.recorder != nil {
    branchRecorder, err = h.recorder.Fork(ctx)
    if err != nil {
        slog.Warn("cxdb fork failed", "error", err)
    }
}
// Pass branchRecorder to child execution
```

#### Phase 5: Finalize

```go
// engine.go finalize()

// After events are closed
if e.Recorder != nil {
    e.Recorder.Close()
}
```

### final.json Output

Write machine-readable output including CXDB metadata:

```go
// engine.go finalize()

if e.Recorder != nil {
    finalData := map[string]any{
        "run_id":          e.RunID,
        "status":          lastStatus,
        "cxdb_context_id": e.Recorder.ContextID(),
        "logs_root":       e.RunDir.Path(),
    }
    writeJSON(filepath.Join(e.RunDir.Path(), "final.json"), finalData)
}
```

## CLI Changes

```go
// cmd/dfgo/main.go

var cxdbAddr string
flag.StringVar(&cxdbAddr, "cxdb", "", "CXDB server address (e.g., localhost:9009)")

cfg := attractor.EngineConfig{
    // ... existing ...
    CXDBAddr: cxdbAddr,
}
```

Also read from environment: `DFGO_CXDB_ADDR`.

## Resume from CXDB

Future enhancement (Phase 3 in improvement plan). On resume:

1. Load checkpoint from filesystem (existing)
2. Connect to CXDB, get context head: `client.GetHead(ctx, contextID)`
3. Fetch recent turns: `client.GetLast(ctx, contextID, limit)`
4. Walk turns backward to reconstruct any state missed by the filesystem checkpoint
5. Continue execution

This requires storing the CXDB context ID in `checkpoint.json`:

```go
type Checkpoint struct {
    // ... existing fields ...
    CXDBContextID uint64 `json:"cxdb_context_id,omitempty"`
    CXDBHeadTurn  uint64 `json:"cxdb_head_turn,omitempty"`
}
```

## Fail-Fast Behavior

Per the metaspec, CXDB must be reachable if configured:

```go
func New(ctx context.Context, cfg Config) (*Recorder, error) {
    client, err := cxdb.Dial(cfg.Address, ...)
    if err != nil {
        return nil, fmt.Errorf("cxdb unreachable at %s: %w", cfg.Address, err)
    }
    // ... context creation ...
}
```

If `CXDBAddr` is set but unreachable, `engine.initialize()` returns an error
and the pipeline does not start.

## Dependencies

Add to `go.mod`:

```
require (
    github.com/strongdm/cxdb/clients/go v0.x.x
    github.com/vmihailenco/msgpack/v5 v5.4.1  // transitive via cxdb client
    github.com/zeebo/blake3 v0.2.4             // transitive via cxdb client
)
```

Since the CXDB repo is local at `~/Work/src/cxdb`, use a replace directive
during development:

```
replace github.com/strongdm/cxdb/clients/go => ../cxdb/clients/go
```

## Testing Strategy

### Unit Tests (recorder_test.go)

Mock the CXDB client interface to test:
- Event-to-turn mapping for all 15 pipeline event types
- Event-to-turn mapping for agent events
- Fork creates new recorder with different context ID
- Encode/decode round-trip for all turn structs
- Error handling (append failures logged, not fatal)

### Integration Tests

- Start CXDB in Docker (`docker run -p 9009:9009 -p 9010:9010 cxdb/cxdb:latest`)
- Run a simple pipeline with `--cxdb localhost:9009`
- Verify context created with correct turns via HTTP API
- Verify turn count matches event count
- Verify parallel branches create forked contexts

### Manual Verification

- Run pipeline, then browse `http://localhost:9010` to see turns in CXDB UI
- Verify typed turns render correctly with field names (requires registry bundle)

## Implementation Order

1. **Scaffold package** -- `cxdbstore/` with Recorder struct, Config, New(), Close()
2. **Turn types** -- All structs with msgpack tags
3. **Event mapping** -- `eventToTurn()` and `agentEventToTurn()`
4. **Engine wiring** -- EngineConfig.CXDBAddr, initialize/finalize hooks
5. **Pipeline event recording** -- Events.On(rec.OnEvent)
6. **Agent event forwarding** -- WithCXDBRecorder option, handler injection
7. **Parallel fork** -- Fork() in parallel handler
8. **Registry bundle** -- Publish on first connect
9. **CLI flag** -- `--cxdb` flag + env var
10. **final.json** -- Write CXDB context ID
11. **Tests** -- Unit + integration
12. **Checkpoint CXDB fields** -- Store context ID for future resume
