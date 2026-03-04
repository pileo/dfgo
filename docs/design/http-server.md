# HTTP Server

**Package**: `internal/server`

The HTTP server exposes a REST API for submitting pipelines, streaming events via SSE, answering human gates, and inspecting run state. It uses only stdlib `net/http` (Go 1.23 `ServeMux` supports `METHOD /path/{param}` routing natively).

## Quick Start

```bash
dfgo serve --addr :8080 --verbose
```

```bash
# Submit a pipeline
curl -X POST localhost:8080/api/v1/pipelines \
  -H 'Content-Type: application/json' \
  -d '{"dot_source":"digraph { start [shape=Mdiamond]; end [shape=Msquare]; start -> end }","auto_approve":true}'

# Stream events (SSE)
curl -N localhost:8080/api/v1/pipelines/<run_id>/events

# Check status
curl localhost:8080/api/v1/pipelines/<run_id>
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check |
| `POST` | `/api/v1/pipelines` | Submit a pipeline, returns run ID |
| `GET` | `/api/v1/pipelines/{id}` | Run status + checkpoint data |
| `GET` | `/api/v1/pipelines/{id}/events` | SSE event stream (with replay) |
| `POST` | `/api/v1/pipelines/{id}/cancel` | Cancel a running pipeline |
| `GET` | `/api/v1/pipelines/{id}/questions` | List pending human gate questions |
| `POST` | `/api/v1/pipelines/{id}/questions/{qid}/answer` | Answer a pending question |
| `GET` | `/api/v1/pipelines/{id}/context` | Runtime context snapshot (KV + logs) |

## Submit Pipeline

**`POST /api/v1/pipelines`**

Request body:

```json
{
  "dot_source": "digraph { ... }",
  "initial_context": {"key": "value"},
  "auto_approve": false,
  "simulate": {
    "rules": [{"node_id": "A", "response": "done"}],
    "fallback": "ok"
  }
}
```

- `dot_source` (required): DOT graph source text.
- `initial_context` (optional): Seed key-value pairs for the pipeline context.
- `auto_approve` (optional): If true, all human gates are auto-approved.
- `simulate` (optional): Simulation config — replaces LLM-backed handlers with deterministic responses. Per-request config takes priority over the server-level `--simulate` flag. See [simulation.md](simulation.md).

Response (202 Accepted):

```json
{"run_id": "uuid"}
```

The pipeline is validated synchronously during submission. If the DOT source is invalid, returns 422 with error details. Execution launches in a background goroutine.

## Run Status

**`GET /api/v1/pipelines/{id}`**

Response:

```json
{
  "run_id": "uuid",
  "status": "running",
  "pipeline": "my_pipeline",
  "current_node": "review",
  "started_at": "2026-03-03T10:30:00Z",
  "completed_at": "2026-03-03T10:31:00Z",
  "error": ""
}
```

Status values: `running`, `completed`, `failed`, `canceled`.

## SSE Event Stream

**`GET /api/v1/pipelines/{id}/events`**

Returns a `text/event-stream` connection. Late-joining clients receive all buffered history events (up to 1024) before live events.

Event format:

```
event: stage.started
data: {"type":"stage.started","timestamp":"2026-03-03T10:30:00Z","data":{"node_id":"plan"}}
id: 7

```

When the run completes, a terminal `event: done` is sent and the stream closes.

Supports the `Last-Event-ID` header for reconnection — events up to that ID are skipped.

Event types are the same as the engine's event system (see `internal/attractor/events`): `pipeline.started`, `pipeline.completed`, `pipeline.failed`, `stage.started`, `stage.completed`, `stage.failed`, `stage.retrying`, `parallel.*`, `interview.*`, `checkpoint.saved`.

## Human Gate Interaction

**`GET /api/v1/pipelines/{id}/questions`**

Lists all pending human gate questions:

```json
[
  {
    "id": "question-uuid",
    "type": "yes_no",
    "prompt": "Approve the changes?",
    "options": ["approve", "reject"],
    "default": ""
  }
]
```

Question types: `yes_no`, `multiple_choice`, `freeform`, `confirmation`.

**`POST /api/v1/pipelines/{id}/questions/{qid}/answer`**

```json
{"text": "yes", "selected": -1}
```

- `text`: The answer text.
- `selected`: Option index for multiple choice (-1 otherwise).

Answering a question unblocks the engine goroutine waiting on that gate. If `auto_approve` was set on submission, no questions will appear.

## Runtime Context

**`GET /api/v1/pipelines/{id}/context`**

Returns the pipeline's current key-value state and log entries:

```json
{
  "run_id": "uuid",
  "context": {"goal": "...", "current_node": "review", "outcome": "SUCCESS"},
  "logs": ["entry 1", "entry 2"]
}
```

## Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  HTTP Client  │────▶│    Server     │────▶│  RunManager   │
└──────────────┘     │  (handlers)   │     │  (lifecycle)  │
                     └──────────────┘     └───────┬──────┘
                                                   │
                                    ┌──────────────┼──────────────┐
                                    ▼              ▼              ▼
                              ┌──────────┐  ┌───────────┐  ┌────────────┐
                              │  Engine   │  │Broadcaster│  │HTTP        │
                              │(Prepare + │  │  (SSE)    │  │Interviewer │
                              │ Execute)  │  └───────────┘  └────────────┘
                              └──────────┘
```

### Key Components

- **Server** (`server.go`): Wraps `http.Server` + `RunManager`. Handles ListenAndServe, Shutdown, and request logging middleware.
- **RunManager** (`runmgr/manager.go`): Tracks concurrent runs. `Submit()` validates synchronously via `engine.Prepare()`, then launches `engine.Execute()` in a goroutine.
- **Broadcaster** (`sse/broadcaster.go`): Per-run fan-out with 1024-event replay buffer. Registered as an `events.Callback` on the engine's emitter.
- **HTTP Interviewer** (`interviewer/http.go`): Channel-per-question bridge. `Ask()` blocks the engine goroutine; `SubmitAnswer()` unblocks it from the HTTP handler.

### Run Lifecycle

1. `POST /api/v1/pipelines` → `RunManager.Submit()`
2. Generate run ID, create Broadcaster + HTTP Interviewer
3. Build engine config, create engine
4. `engine.Prepare()` — parse, validate, initialize (synchronous, returns error on invalid DOT)
5. Register `broadcaster.Publish` on `engine.Events`
6. Launch `engine.Execute()` in goroutine
7. Return run ID immediately (202 Accepted)
8. Engine runs, events flow through broadcaster to SSE clients
9. On completion/failure/cancel: update run status, close broadcaster

### Context Management

Each run gets a detached context (`context.Background()`) so that the HTTP request context ending doesn't cancel the run. Cancellation is explicit via `POST /cancel` or server shutdown.

## CLI Flags

```
dfgo serve [flags]

Flags:
  --addr string       listen address (default ":8080")
  --logs-dir string   directory for run logs (default "runs")
  --verbose           enable verbose (DEBUG) logging
  --cxdb string       CXDB server address (e.g., localhost:9009)
  --simulate string   simulation config JSON file (bypasses LLM calls)
```

The serve command sets up LLM clients from environment variables (same as `dfgo run`) and performs graceful shutdown on SIGINT: drains SSE connections, cancels running pipelines, waits for completion.

## Error Responses

All errors use a standard envelope:

```json
{
  "error": "human-readable message",
  "details": "additional context"
}
```

| Status | Condition |
|--------|-----------|
| 400 | Invalid JSON, missing required fields |
| 404 | Run or question not found |
| 422 | Invalid DOT source (parse/validation failure) |
| 500 | Internal server error |
