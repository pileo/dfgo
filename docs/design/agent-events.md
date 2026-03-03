# Agent Events

**Package**: `internal/agent/event`

A typed event system for observing agent execution. Events are dispatched asynchronously via a buffered channel to decouple the agent loop from slow listeners.

## Emitter

```go
emitter := event.NewEmitter(256)  // buffer capacity

emitter.On(func(e event.Event) {
    log.Printf("[%s] %s %v", e.Timestamp.Format(time.RFC3339), e.Type, e.Data)
})

emitter.Emit(event.ToolStart, map[string]any{"tool": "shell", "call_id": "tc_1"})
emitter.Close()  // drains pending events, then returns
```

`Emit` is non-blocking: if the buffer is full, the event is dropped rather than blocking the agent loop. `Close` shuts down the drain goroutine and waits for all pending events to be delivered.

## Event Types

| Type | Emitted when |
|---|---|
| `session.start` | Session begins |
| `session.end` | Session completes |
| `turn.start` | New LLM round-trip begins |
| `turn.end` | LLM round-trip completes (after tool execution) |
| `llm.request` | Before calling `client.Complete()` |
| `llm.response` | After receiving a response |
| `llm.error` | LLM call fails |
| `tool.start` | Before executing a tool |
| `tool.end` | After tool execution completes |
| `tool.error` | Tool execution fails |
| `steering` | Steering message injected |
| `loop.detected` | Repetitive tool call pattern detected |
| `context.truncate` | Context window utilization exceeds 80% |
| `llm.stream.start` | Streaming response opened (when `Streaming` is enabled) |
| `llm.chunk` | Content delta received during streaming |
| `llm.stream.end` | Streaming response complete |
| `abort` | Session aborted (timeout, max rounds, cancellation) |

## Event Structure

```go
type Event struct {
    Type      Type
    Timestamp time.Time
    Data      map[string]any
}
```

`Data` contents vary by event type. Common keys include `tool`, `call_id`, `model`, `round`, `error`, `finish_reason`, `input_tokens`, `output_tokens`.

### Streaming Events

When `Config.Streaming` is enabled, `llm.request`/`llm.response` are still emitted but the LLM call also produces streaming-specific events:

| Event | Data keys | Description |
|---|---|---|
| `llm.stream.start` | `response_id`, `model` | Stream opened, response metadata available |
| `llm.chunk` | `kind`, `text`, `index` | Content delta — `kind` is `text`, `tool_call`, or `thinking` |
| `llm.stream.end` | `finish_reason`, `input_tokens`, `output_tokens` | Stream complete, final usage available |

These events enable real-time rendering of model output (e.g., printing text deltas as they arrive).
