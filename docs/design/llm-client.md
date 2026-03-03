# Unified LLM Client

**Package**: `internal/llm` and `internal/llm/provider`

A multi-provider LLM client that routes completion requests to Anthropic, OpenAI, or Gemini via a unified interface. No external SDK dependencies — direct `net/http` + `encoding/json`.

## Architecture

```
Request → Client → Middleware chain → ProviderAdapter → HTTP API
                                                          ↓
Response ← Client ← Middleware chain ← ProviderAdapter ← HTTP API
```

The client holds a map of named provider adapters and applies middleware in onion order (first registered = outermost wrapper).

## Types

Messages use a multimodal content model supporting text, images, tool calls, tool results, and thinking blocks:

```go
type Message struct {
    Role       Role          // system, user, assistant, tool, developer
    Content    []ContentPart
    Name       string
    ToolCallID string        // for tool-role messages
}

type ContentPart struct {
    Kind       ContentKind   // text, image, tool_call, tool_result, thinking
    Text       string
    ToolCall   *ToolCallData
    ToolResult *ToolResultData
    Thinking   *ThinkingData
}
```

Convenience: `TextMessage(role, text)` creates a single-text message. `Message.Text()` concatenates text parts. `Message.ToolCalls()` extracts tool call parts.

## Request / Response

```go
type Request struct {
    Model           string
    Messages        []Message
    Provider        string         // optional: route to specific provider
    Tools           []ToolDef
    ToolChoice      *ToolChoice    // auto/none/required/named
    Temperature     *float64
    MaxTokens       *int
    ReasoningEffort string         // low/medium/high (OpenAI reasoning models)
    ProviderOptions map[string]any // escape hatch
}

type Response struct {
    ID, Model, Provider string
    Message             Message
    FinishReason        FinishReason  // stop, tool_use, length, content_filter
    Usage               Usage
    Raw                 json.RawMessage
}
```

## Client

```go
client := llm.NewClient(
    llm.WithProvider(provider.NewAnthropic()),
    llm.WithProvider(provider.NewOpenAI()),
    llm.WithRetry(llm.DefaultRetryPolicy()),
    llm.WithLogging(slog.Default()),
)

resp, err := client.Complete(ctx, llm.Request{
    Model:    "claude-sonnet-4-20250514",
    Messages: []llm.Message{llm.TextMessage(llm.RoleUser, "Hello")},
})
```

If `Request.Provider` is set, routes to that specific adapter. Otherwise uses the default (first registered).

## Provider Adapters

All implement `ProviderAdapter`:

```go
type ProviderAdapter interface {
    Name() string
    Complete(ctx context.Context, req Request) (*Response, error)
}
```

### Anthropic (`provider.NewAnthropic()`)

- POST to `https://api.anthropic.com/v1/messages`
- System messages extracted to top-level `system` parameter
- Tool results sent as `tool_result` content blocks in user messages
- Thinking blocks round-tripped verbatim (with `signature` for caching)
- `max_tokens` required (defaults to 4096)
- API key from `ANTHROPIC_API_KEY` env var

### OpenAI (`provider.NewOpenAI()`)

- POST to `https://api.openai.com/v1/responses` (Responses API, not Chat Completions)
- System messages sent as `instructions` parameter
- `reasoning.effort` mapped from `ReasoningEffort` field
- Function calls represented as `function_call` output items
- API key from `OPENAI_API_KEY` env var

### Gemini (`provider.NewGemini()`)

- POST to `https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`
- System messages sent as `systemInstruction`
- No native tool call IDs — adapter generates synthetic UUIDs
- API key from `GEMINI_API_KEY` env var (passed as query parameter)

## Streaming

The client supports streaming responses via a scanner-pattern API. Providers that support streaming implement the optional `StreamingProvider` interface; those that don't fall back to `Complete()` with a synthesized single-event stream.

### Usage

```go
stream, err := client.Stream(ctx, llm.Request{
    Model:    "claude-sonnet-4-20250514",
    Messages: []llm.Message{llm.TextMessage(llm.RoleUser, "Hello")},
})
if err != nil { ... }
defer stream.Close()

for stream.Next() {
    evt := stream.Event()
    switch evt.Type {
    case llm.EventResponseMeta:
        fmt.Printf("model=%s id=%s\n", evt.Model, evt.ResponseID)
    case llm.EventContentDelta:
        fmt.Print(evt.Text) // real-time text output
    }
}
if err := stream.Err(); err != nil { ... }
resp := stream.Response() // fully accumulated *Response
```

### StreamingProvider Interface

```go
type StreamingProvider interface {
    ProviderAdapter
    CompleteStream(ctx context.Context, req Request) (*Stream, error)
}
```

All three providers (Anthropic, OpenAI, Gemini) implement `StreamingProvider`. When `Client.Stream()` is called, it checks if the resolved (middleware-wrapped) adapter implements this interface. If not, it falls back to `Complete()` and wraps the result via `CompleteToStream()`.

### Stream Events

Events follow a content-block lifecycle: `content.start` → one or more `content.delta` → `content.stop`, with metadata events at the start and end.

| Type | Fields | Description |
|---|---|---|
| `response.meta` | `ResponseID`, `Model` | Stream opened, response ID and model known |
| `content.start` | `Index`, `ContentKind`, `ToolCallID`?, `ToolName`? | New content block started |
| `content.delta` | `Index`, `ContentKind`, `Text` | Incremental content (text, tool args JSON, thinking) |
| `content.stop` | `Index`, `ContentKind` | Content block complete |
| `usage` | `Usage`, `FinishReason` | Token counts and stop reason |
| `error` | `Err` | Provider-side stream error |

`ContentKind` matches the non-streaming types: `text`, `tool_call`, `thinking`.

### SSE Parser

All three providers use Server-Sent Events (SSE). A shared `SSEScanner` (`internal/llm/sse.go`) parses the `text/event-stream` format per the W3C spec:

```go
scanner := llm.NewSSEScanner(httpResp.Body)
for scanner.Next() {
    sse := scanner.Event() // .Event, .Data, .ID fields
}
if err := scanner.Err(); err != nil && err != io.EOF { ... }
```

Handles multi-line `data:` fields, comments, CRLF line endings, and EOF-terminated streams.

### Provider Streaming Details

**Anthropic** (`anthropic_stream.go`): Adds `"stream": true` to the request body. Translates SSE events: `message_start` → `EventResponseMeta`, `content_block_start/delta/stop` → content lifecycle events, `message_delta` → `EventUsage`, `message_stop` → finish. Handles thinking blocks with signature accumulation, and tool_use blocks with incremental JSON input.

**OpenAI** (`openai_stream.go`): Adds `"stream": true` to the Responses API request. Translates SSE events: `response.created` → `EventResponseMeta`, `response.output_item.added`/`response.content_part.added` → `EventContentStart`, `response.output_text.delta`/`response.function_call_arguments.delta` → `EventContentDelta`, `response.output_item.done` → `EventContentStop`, `response.completed` → `EventUsage` + finish.

**Gemini** (`gemini_stream.go`): Uses `streamGenerateContent?alt=sse` endpoint. Each SSE chunk is a complete `geminiResponse` JSON. Diffs successive chunks to emit content start/delta/stop events. Generates synthetic UUIDs for tool call IDs (same as non-streaming). Final chunk with `finishReason` set triggers usage and stream finish.

### Fallback

`CompleteToStream(resp, err)` wraps any `*Response` as a fully-buffered stream, emitting the standard event sequence (meta → start/delta/stop per block → usage). Used when a provider doesn't implement `StreamingProvider` or when middleware needs a fallback.

## Error Hierarchy

All errors embed `SDKError` and support `errors.As()` unwrapping:

| Type | HTTP Status | Retryable |
|---|---|---|
| `AuthenticationError` | 401, 403 | No |
| `RateLimitError` | 429 | Yes |
| `InvalidRequestError` | 400 | No |
| `ContextLengthError` | 413 | No |
| `ServerError` | 5xx | Yes |
| `NetworkError` | (connection failure) | Yes |
| `RequestTimeoutError` | (deadline exceeded) | Yes |
| `AbortError` | (context cancelled) | No |

`llm.IsRetryable(err)` walks the error chain and returns true for retryable errors.

`llm.NewProviderError(provider, statusCode, message, cause)` classifies by HTTP status automatically.

## Middleware

Middleware wraps a `ProviderAdapter` with cross-cutting behavior. Both middleware adapters implement `StreamingProvider` when the inner adapter does, forwarding `CompleteStream()` calls through the chain.

- **RetryMiddleware**: exponential backoff with optional jitter. Only retries errors where `IsRetryable()` returns true. Respects context cancellation during backoff. For streaming, retries connection-level errors (before first event); once a stream is open, no retry is possible.
- **LoggingMiddleware**: logs request metadata and response metrics at `slog.Debug` level, errors at `slog.Warn`. For streaming, logs stream start and delegates to the inner adapter.

```go
llm.RetryMiddleware(llm.RetryPolicy{
    MaxRetries: 3,
    BaseDelay:  500 * time.Millisecond,
    MaxDelay:   30 * time.Second,
    Jitter:     true,
})
```

## Model Catalog

A static registry of known models with metadata (`catalog.go`). Provides context window sizes, max output tokens, capability flags, and per-token costs without querying any external API.

### Types

```go
type Capability string // "tools", "vision", "streaming", "thinking", "caching"

type ModelInfo struct {
    ID              string
    Provider        string       // "anthropic", "openai", "gemini"
    DisplayName     string
    ContextWindow   int          // max input tokens
    MaxOutputTokens int          // max output tokens (0 = provider default)
    Capabilities    []Capability
    InputCostPer1M  float64      // USD per 1M input tokens
    OutputCostPer1M float64      // USD per 1M output tokens
}
```

`ModelInfo.HasCapability(cap)` checks for a specific capability.

### Lookup Functions

| Function | Description |
|---|---|
| `GetModelInfo(modelID)` | Returns metadata for a model ID, or false if unknown |
| `ListModels(provider)` | Returns all models, optionally filtered by provider (empty string = all) |
| `GetLatestModel(provider)` | Returns the recommended latest model for a provider |

### Known Models

| Provider | Models |
|---|---|
| Anthropic | claude-opus-4-6, claude-sonnet-4-6, claude-haiku-4-5-20251001 |
| OpenAI | o3, o4-mini, gpt-4.1, gpt-4.1-mini |
| Gemini | gemini-2.5-pro, gemini-2.5-flash |
