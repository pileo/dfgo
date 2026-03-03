# Prompt: Plan Client.Stream() for dfgo

> **Status: COMPLETED** — All phases implemented and verified. See design docs:
> `docs/design/llm-client.md`, `docs/design/agent-loop.md`, `docs/design/agent-events.md`

## Task

Plan the implementation of `Client.Stream()` for the unified LLM client in dfgo, and integrate it into the agent loop. This is the streaming counterpart to the existing `Client.Complete()`. The agent loop should be able to use either method, with streaming as the default for interactive use.

## Context: What exists today

dfgo has a unified LLM client (`internal/llm/`) with three provider adapters (Anthropic, OpenAI, Gemini) and a coding agent loop (`internal/agent/`) that calls `Client.Complete()` in a turn loop. All providers use direct `net/http` — no external SDK dependencies.

### Current Client.Complete() flow

```
Session.Run(ctx, input)
  → buildRequest() → llm.Request
  → client.Complete(ctx, req) → blocks until full response → *llm.Response
  → accumulate usage, record assistant message
  → if tool calls: execute tools, loop back
  → if no tool calls: return Result
```

### Key types the stream must integrate with

```go
// internal/llm/provider.go
type ProviderAdapter interface {
    Name() string
    Complete(ctx context.Context, req Request) (*Response, error)
}

// internal/llm/request.go
type Response struct {
    ID, Model, Provider string
    Message             Message       // accumulated content parts
    FinishReason        FinishReason  // stop, tool_use, length, content_filter
    Usage               Usage
    Raw                 json.RawMessage
}

// internal/llm/types.go — content model
type Message struct { Role; Content []ContentPart; Name; ToolCallID }
type ContentPart struct { Kind; Text; ToolCall *ToolCallData; ToolResult *ToolResultData; Thinking *ThinkingData }
// Kinds: text, image, tool_call, tool_result, thinking

// internal/llm/middleware.go
type Middleware func(ProviderAdapter) ProviderAdapter
// Existing: RetryMiddleware, LoggingMiddleware — both wrap Complete() only today
```

### Agent event system

The agent emits events via a buffered channel (cap 256). Current event types:

```
session.start, session.end, turn.start, turn.end,
llm.request, llm.response, llm.error,
tool.start, tool.end, tool.error,
steering, loop.detected, context.truncate, abort
```

### Provider SSE formats

**Anthropic** (`POST /v1/messages` with `"stream": true`):
- `message_start` → contains message id, model, usage.input_tokens
- `content_block_start` → new content block (text, tool_use, thinking)
- `content_block_delta` → incremental text, partial tool input JSON, thinking text
- `content_block_stop` → block complete
- `message_delta` → stop_reason, usage.output_tokens
- `message_stop` → end of stream
- Events are `text/event-stream` with `event:` and `data:` fields

**OpenAI** (`POST /v1/responses` with `"stream": true`):
- `response.created` → response object with id, model
- `response.output_item.added` → new output item (message, function_call, reasoning)
- `response.content_part.added` → new content part
- `response.output_text.delta` → text delta
- `response.function_call_arguments.delta` → tool call argument delta
- `response.output_item.done` → item complete with full content
- `response.completed` → final response with usage
- Events are `text/event-stream`

**Gemini** (`POST /v1beta/models/{model}:streamGenerateContent?alt=sse`):
- Each SSE `data:` line is a complete `GenerateContentResponse` JSON (same schema as non-streaming, but partial)
- Successive chunks may contain new `candidates[0].content.parts` entries or updated `usageMetadata`
- Final chunk has `candidates[0].finishReason` set

## Design constraints

1. **No external dependencies** — continue using `net/http` + `encoding/json` + `bufio` for SSE parsing
2. **Middleware must work with streaming** — the existing `Middleware func(ProviderAdapter) ProviderAdapter` pattern wraps `Complete()`. Streaming needs middleware support too (at minimum: logging of stream start/end/errors, retry on connection failures before first chunk)
3. **Backward compatible** — `Complete()` must continue to work unchanged. Providers that don't support streaming should fall back to `Complete()` + synthesized stream.
4. **Agent loop integration** — the session should emit chunk-level events so callers can render real-time output, but the core loop logic (tool execution, steering injection, loop detection) stays the same
5. **Tool call accumulation** — tool call arguments arrive as deltas (partial JSON). The stream consumer must accumulate them and only dispatch tool execution once the call is fully formed (on `content_block_stop` / `response.output_item.done`)
6. **Thinking block round-tripping** — Anthropic thinking blocks stream incrementally with a signature at the end. Must accumulate and preserve signatures for cache-aware round-tripping.
7. **Error mid-stream** — if the connection drops or the provider sends an error event mid-stream, surface it cleanly without leaving the session in a broken state
8. **Usage reporting** — all three providers report usage at different points in the stream. Must accumulate correctly (input_tokens often comes at stream start, output_tokens at stream end)

## What to produce

A phased implementation plan covering:

1. ~~**New types**~~ **DONE** — Scanner-pattern `Stream` type (`stream.go`), `StreamEvent` with 6 event types, `StreamEventType` constants. Chose iterator over channels/callbacks for idiomatic Go, no goroutine leak risk.
2. ~~**ProviderAdapter changes**~~ **DONE** — Optional `StreamingProvider` interface (`provider.go`) with `CompleteStream()` method. Backward compatible; providers that don't implement it fall back to `Complete()` + `CompleteToStream()`.
3. ~~**SSE parser**~~ **DONE** — Shared `SSEScanner` (`sse.go`) following W3C spec. Handles multi-line data, comments, CRLF, EOF edge cases.
4. ~~**Provider implementations**~~ **DONE** — All three providers implement `StreamingProvider`: `anthropic_stream.go` (message_start/content_block lifecycle/message_delta/message_stop), `openai_stream.go` (response.created/output_item/content_part/completed), `gemini_stream.go` (chunk diffing with streamGenerateContent?alt=sse).
5. ~~**Middleware adaptation**~~ **DONE** — Both `loggingAdapter` and `retryAdapter` implement `CompleteStream()` via type-assert forwarding. Retry only before first event; falls back to `Complete()` when inner adapter doesn't support streaming.
6. ~~**Client.Stream()**~~ **DONE** — Same provider resolution + middleware wrapping as `Complete()`, then type-asserts for `StreamingProvider`. Falls back to `CompleteToStream()`.
7. ~~**Agent loop changes**~~ **DONE** — `Config.Streaming` bool field, `streamTurn()` method in `session.go`, conditional branching in `Run()`. Accumulated `*Response` feeds into the same turn logic unchanged.
8. ~~**New events**~~ **DONE** — 3 event types: `llm.stream.start` (response_id, model), `llm.chunk` (kind, text, index), `llm.stream.end` (finish_reason, tokens).
9. ~~**Testing strategy**~~ **DONE** — 33 new tests: SSE parser (10), Stream type (4), Anthropic stream (5), OpenAI stream (3), Gemini stream (3), Client.Stream routing (5), agent session streaming (3). All use mock SSE httptest servers.

For each phase, identify the files created/modified and key design decisions with rationale.

## Files created/modified

| Action | File | What |
|--------|------|------|
| new | `internal/llm/stream.go` | `Stream`, `StreamEvent`, `StreamEventType`, `CompleteToStream` |
| new | `internal/llm/sse.go` | `SSEScanner`, `SSEEvent` |
| new | `internal/llm/sse_test.go` | 10 SSE parser tests |
| new | `internal/llm/stream_test.go` | 4 Stream tests |
| mod | `internal/llm/provider.go` | `StreamingProvider` interface |
| mod | `internal/llm/client.go` | `Client.Stream()` method |
| mod | `internal/llm/middleware.go` | `CompleteStream()` on both adapters |
| mod | `internal/llm/client_test.go` | 5 Client.Stream tests |
| new | `internal/llm/provider/anthropic_stream.go` | Anthropic streaming |
| new | `internal/llm/provider/openai_stream.go` | OpenAI streaming |
| new | `internal/llm/provider/gemini_stream.go` | Gemini streaming |
| new | `internal/llm/provider/anthropic_stream_test.go` | 5 tests |
| new | `internal/llm/provider/openai_stream_test.go` | 3 tests |
| new | `internal/llm/provider/gemini_stream_test.go` | 3 tests |
| mod | `internal/agent/config.go` | `Streaming bool` field |
| mod | `internal/agent/event/event.go` | 3 new event types |
| mod | `internal/agent/session.go` | `streamTurn()`, streaming branch in `Run()` |
| mod | `internal/agent/session_test.go` | 3 agent streaming tests |

## References

- Existing implementation: `internal/llm/`, `internal/agent/`
- Design docs: `docs/design/llm-client.md`, `docs/design/agent-loop.md`
- Anthropic streaming docs: https://docs.anthropic.com/en/api/messages-streaming
- OpenAI streaming docs: https://platform.openai.com/docs/api-reference/responses-streaming
