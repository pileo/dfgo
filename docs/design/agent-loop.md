# Agent Loop

**Package**: `internal/agent`

An autonomous coding agent that pairs an LLM with developer tools through an agentic loop. The agent uses the unified LLM client's `Client.Complete()` directly, implementing its own turn loop with steering, loop detection, and circuit breaking.

## Session

`Session` is the core type. It takes an `*llm.Client`, a `profile.Profile`, and an `execenv.Environment`, and translates between agent-level messages and LLM SDK requests.

```go
session := agent.NewSession(agent.Config{
    Client:    llmClient,
    Profile:   profile.Anthropic{},
    Env:       execenv.NewLocal("/path/to/project"),
    Model:     "claude-sonnet-4-20250514",
    MaxRounds: 200,
})

result := session.Run(ctx, "Fix the failing test in main_test.go")
fmt.Println(result.FinalText)
fmt.Println(result.Rounds, result.TotalUsage)
```

## Core Loop

The loop runs until the model responds without tool calls (natural exit) or a limit is hit:

```
User input
  ↓
[Build system prompt + message history]
  ↓
[Streaming?] ──Yes──→ [Call client.Stream() → streamTurn()]
  │                              ↓
  No                    [Emit stream events, accumulate response]
  ↓                              ↓
[Call client.Complete()] ←───────┘
  ↓
[Accumulate usage, record assistant message]
  ↓
Tool calls? ──No──→ Return result (natural exit)
  │
  Yes
  ↓
[For each tool call: loop detection → execute → record result]
  ↓
[Inject pending steering messages]
  ↓
[Loop back]
```

### Exit Conditions

| Condition | Result |
|---|---|
| Model responds without tool calls | Natural exit — `result.Aborted = false` |
| `ctx` cancelled | Abort — `result.Aborted = true` |
| `MaxRounds` reached | Abort — `result.Aborted = true` |
| 3 consecutive LLM failures | Error — `result.Error` set (circuit breaker) |
| Non-retryable LLM error | Error — `result.Error` set |

### Context Window Monitoring

After each LLM response, the session estimates context utilization as `input_tokens / profile.ContextWindowSize()`. At 80%, a steering message warns the model to wrap up.

## Agent Messages vs LLM Messages

Agent messages (`internal/agent/message`) are simpler than the LLM SDK's multimodal `ContentPart` model. The session translates between them:

```go
// Agent-level (string content, flat tool calls)
type Message struct {
    Role       Role       // user, assistant, tool, steering
    Content    string
    ToolCalls  []ToolCall
    ToolResult *ToolResult
}
```

Translation rules:
- `user` → `llm.RoleUser` with text content
- `assistant` → `llm.RoleAssistant` with text + tool call parts
- `tool` → `llm.RoleTool` with tool result content
- `steering` → `llm.RoleUser` with `[SYSTEM]` prefix (appears as user message to the model)

## Steering

External code can inject steering messages into the agent loop:

```go
session.Steer("Focus on writing tests, not refactoring")
```

Steering messages are queued and injected after all tool results in the current round. They appear as user messages prefixed with `[SYSTEM]`. The agent also self-steers on loop detection and context window pressure.

## Streaming Mode

When `Config.Streaming` is `true`, the session calls `client.Stream()` instead of `client.Complete()` for each LLM turn. The `streamTurn()` method:

1. Opens a stream via `client.Stream(ctx, req)`
2. Iterates `stream.Next()`, emitting events:
   - `EventResponseMeta` → `llm.stream.start` event (response ID, model)
   - `EventContentDelta` → `llm.chunk` event (content kind, text, block index)
   - `EventError` → returns the error immediately
3. After the stream ends, calls `stream.Response()` for the fully accumulated `*Response`
4. Emits `llm.stream.end` with usage and finish reason

The accumulated response is identical to what `Complete()` would return, so the rest of the turn logic (usage accumulation, message translation, tool execution) stays the same. Streaming is purely additive — it adds real-time events without changing the turn contract.

If the provider doesn't implement `StreamingProvider`, `client.Stream()` falls back to `Complete()` and wraps the result as a synthesized stream. This means `Streaming: true` is always safe to set.

## Result

```go
type Result struct {
    Messages   []message.Message  // full conversation history
    FinalText  string             // assistant's last text response
    TotalUsage llm.Usage          // cumulative token consumption
    Rounds     int                // number of LLM round-trips
    Aborted    bool
    Error      error
}
```

## Configuration

```go
type Config struct {
    Client      *llm.Client
    Profile     profile.Profile
    Env         execenv.Environment
    Model       string
    MaxTurns    int       // total turns (0 = unlimited)
    MaxRounds   int       // LLM round-trips (default 200)
    ProjectDoc  string    // included in system prompt
    UserPrompt  string    // appended to system prompt
    Temperature *float64
    MaxTokens   *int
    Streaming   bool      // use streaming responses (default false)
}
```
