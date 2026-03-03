# Implementation Plan: Attractor Spec Compliance

## Context

The dfgo project implements the Attractor pipeline orchestration engine specified at `github.com/strongdm/attractor`. A gap analysis identified ~35 missing features across three spec documents (attractor-spec, coding-agent-loop-spec, unified-llm-spec). This plan closes those gaps in dependency order across 7 phases.

---

## Phase 1: Core Engine Corrections (Critical) — COMPLETED

These are correctness bugs — the engine behaves differently from the spec today. All 7 items implemented and tested.

### 1.1 Retry backoff delay

**File:** `internal/attractor/engine.go` (lines 217-228)

Currently retries fire instantly with no delay. Spec requires exponential backoff with jitter.

**Changes:**
- Add `RetryPolicy` and `BackoffConfig` types to `internal/attractor/runtime/retry.go` (new file)
  - Fields: `MaxAttempts int`, `InitialDelayMs int` (default 200), `BackoffFactor float64` (default 2.0), `MaxDelayMs int` (default 60000), `Jitter bool` (default true)
  - Add `DelayForAttempt(attempt int) time.Duration` method
  - Add 5 preset constructors: `NonePolicyPreset()`, `StandardPreset()`, `AggressivePreset()`, `LinearPreset()`, `PatientPreset()`
- In `engine.go:execute()`, replace the bare `continue` on retry with a call to `DelayForAttempt` + `time.Sleep` (respecting ctx cancellation via `select`)
- Read `retry_policy` node attr to select preset; default to `StandardPreset()` if node has `max_retries > 0`

### 1.2 `default_max_retry` graph attribute

**File:** `internal/attractor/engine.go` (line 218)

Currently `node.IntAttr("max_retries", 0)` — nodes without `max_retries` get 0 retries. Spec says fallback to graph `default_max_retry` (default 50).

**Change:** Replace the hardcoded `0` default with:
```go
graphDefault := g.IntAttr("default_max_retry", 50)  // need to add IntAttr to Graph
maxRetries := node.IntAttr("max_retries", graphDefault)
```
- Add `IntAttr`/`StringAttr`/`BoolAttr` helper methods on `*model.Graph` (same pattern as Node)

### 1.3 Goal gate enforcement with retry_target chain

**File:** `internal/attractor/engine.go` (lines 187-195, 230-241)

Current: When exit node reached, no goal gate check. When a goal gate node fails, it just errors out.

Spec: At terminal node, check all visited goal_gate nodes. If any non-success, jump to retry_target chain. Failure routing: node `retry_target` -> node `fallback_retry_target` -> graph `retry_target` -> graph `fallback_retry_target` -> pipeline fail.

**Changes:**
- Add `checkGoalGates()` method on Engine that iterates visitLog for goal_gate nodes
- At the exit node break (line 189), call `checkGoalGates()`. If unsatisfied, resolve retry target and set `currentID` to it instead of breaking
- Add `resolveRetryTarget(nodeID string) (string, bool)` that checks node attrs then graph attrs
- On FAIL status (not just goal_gate), implement the failure routing chain:
  1. Check for outgoing edge with `condition="outcome=fail"` (already works via edge.Select)
  2. Check node `retry_target` attr
  3. Check node `fallback_retry_target` attr
  4. Check graph `retry_target` attr
  5. Check graph `fallback_retry_target` attr
  6. Pipeline termination

### 1.4 `allow_partial` node attribute

**File:** `internal/attractor/engine.go` (line 225)

When retries exhausted, if `allow_partial=true`, return PARTIAL_SUCCESS instead of FAIL.

**Change:** After `slog.Warn("max retries exceeded")`:
```go
if node.BoolAttr("allow_partial", false) {
    outcome.Status = runtime.StatusPartialSuccess
    outcome.Notes = "retries exhausted, partial accepted"
} else {
    outcome.Status = runtime.StatusFail
    outcome.FailureReason = "max retries exceeded"
}
```

### 1.5 Built-in context keys

**File:** `internal/attractor/engine.go` (execute loop)

Spec requires engine to set these keys:
- `outcome` — after each handler returns
- `preferred_label` — after each handler returns
- `current_node` — before each handler executes
- `graph.goal` — at initialization (already done as `goal`, change to also set `graph.goal`)
- `internal.retry_count.<node_id>` — when retry counter changes

**Changes:** Add `e.PCtx.Set(...)` calls at the appropriate points in the execute loop.

### 1.6 Fix truncation order

**File:** `internal/agent/tool/truncate/truncate.go`

Current: Line-based first (Phase 1), then character-based (Phase 2).
Spec: Character-based first, then line-based.

**Change:** Swap Phase 1 and Phase 2 in `TruncateWithLimits()`. Character truncation runs first, line truncation second. Update the package doc comment.

### 1.7 Outcome `SuggestedNextIDs` (plural)

**File:** `internal/attractor/runtime/outcome.go`

Spec uses `suggested_next_ids: List<String>`, implementation has `SuggestedNextID string` (singular).

**Changes:**
- Rename field to `SuggestedNextIDs []string` in Outcome
- Update `edge/selector.go` Step 3 to iterate the list
- Update `wait_human.go` to set `SuggestedNextIDs: []string{...}`

---

## Phase 2: Missing Validation Rules — COMPLETED

All 6 missing rules implemented and tested (14 test cases across 6 test functions, all passing).

**Files modified:**
- `internal/attractor/validate/rules.go` — Added 6 new rule types to `BuiltinRules()`
- `internal/attractor/validate/validate.go` — Added `RunnerOption` type, `WithKnownTypes()` option, variadic `NewRunner(opts...)`
- `internal/attractor/validate/validate_test.go` — Added 6 new test functions
- `internal/attractor/handler/handler.go` — Added `KnownTypes()` and `KnownShapes()` methods to Registry
- `internal/attractor/style/stylesheet.go` — Changed `ParseStylesheet` to return `(Stylesheet, error)` for structural error detection
- `internal/attractor/style/stylesheet_test.go` — Updated callers, added `TestParseStylesheetErrors`
- `internal/attractor/engine.go` — Engine `validate()` now passes `WithKnownTypes(registry.KnownTypes())` to the runner

| Rule | Severity | Implementation |
|------|----------|----------------|
| `start_no_incoming` | ERROR | `len(g.InEdges(start.ID)) == 0` |
| `exit_no_outgoing` | ERROR | For each exit node, `len(g.OutEdges(n.ID)) == 0` |
| `stylesheet_syntax` | ERROR | If `g.Attrs["model_stylesheet"] != ""`, call `style.ParseStylesheet()` and check for errors |
| `type_known` | WARNING | Check `n.Attrs["type"]` against known types from `Registry.KnownTypes()`; only active when `WithKnownTypes` option provided |
| `fidelity_valid` | WARNING | Checks `fidelity` attr on nodes, edges, and graph via `fidelity.Mode(v).Valid()` |
| `retry_target_exists` | WARNING | If node has `retry_target` or `fallback_retry_target`, check `g.NodeByID()` exists |

Also fix `reachability` severity from WARNING to ERROR per spec. — **COMPLETED** (done in Phase 1)

---

## Phase 3: Handler Completions — COMPLETED

### 3.1 Fan-in handler — heuristic ranking — COMPLETED

**File:** `internal/attractor/handler/fan_in.go`

Implemented spec algorithm:
1. Reads `parallel.results` from context (JSON map of nodeID -> outcome)
2. Heuristic ranking: sort by status priority (SUCCESS=0, PARTIAL_SUCCESS=1, RETRY=2, FAIL=3), then by id ascending
3. Writes `parallel.fan_in.best_id` and `parallel.fan_in.best_outcome` to context
4. Returns SUCCESS (or FAIL if all candidates failed)

Tests: 5 test functions (no results, ranking, all-failed, tiebreak-by-ID, invalid JSON).

### 3.2 Parallel handler — `error_policy` and `max_parallel` — COMPLETED

**File:** `internal/attractor/handler/parallel.go`

Implemented:
- `error_policy` attr: `"continue"` (default), `"fail_fast"` (context cancellation), `"ignore"` (filter failures from join)
- `max_parallel` attr (default 4): channel-based semaphore limiting concurrent goroutines
- Stores branch results in context as `parallel.results` (JSON) for fan-in consumption

Tests: 5 test functions (stores results, max_parallel concurrency, fail_fast, ignore, ignore-all-fail).

### 3.3 Manager loop handler — supervision loop — COMPLETED

**File:** `internal/attractor/handler/manager_loop.go`

Implemented full supervision loop:
- Reads `manager.poll_interval` (duration parsing incl. "d" suffix, default "45s"), `manager.max_cycles` (default 1000), `manager.stop_condition`, `manager.actions` (default "observe,wait")
- Observation loop: checks `stack.child.status` context key, evaluates custom stop_condition via `cond.Eval`
- Wait action: `time.After(pollInterval)` with ctx cancellation via `select`
- Returns SUCCESS on child completion/stop condition, FAIL on max_cycles or child failure

Tests: 7 test functions (max cycles, stop condition, child status, child fail, cancellation, observe-only, invalid stop condition) + `TestParseDuration` (8 subtests).

### 3.4 Tool handler — `tool_command` attr name — COMPLETED

> Implemented in Phase 1 as a bonus fix.

**File:** `internal/attractor/handler/tool.go` (line 18)

Spec says `tool_command`, implementation reads `command`. Support both for backward compatibility:
```go
cmdStr := node.StringAttr("tool_command", node.StringAttr("command", ""))
```

---

## Phase 4: Engine Features — COMPLETED

### 4.1 Stylesheet application transform — COMPLETED

**Files modified:**
- `internal/attractor/engine.go` — Added `applyStylesheet()` method called between parse and validate
- `internal/attractor/style/stylesheet.go` — Added `Apply(g)` method, extended `Matches()` to check `n.Attrs["class"]` (comma-split)

Tests: 3 new test functions in stylesheet_test.go (class matching, Apply, no-override-explicit)

### 4.2 Per-node status.json — COMPLETED

**File modified:** `internal/attractor/engine.go`

Added `writeNodeStatus()` method and `nodeStatus` struct. Called after every non-terminal node execution. Writes JSON with outcome, preferred_next_label, suggested_next_ids, context_updates, notes.

Tests: `TestNodeStatusJSON` in engine_test.go

### 4.3 Observability events — COMPLETED

**Files created:**
- `internal/attractor/events/events.go` — 15 event types, Event struct, Emitter (buffered channel pattern from agent/event)

**Files modified:**
- `internal/attractor/engine.go` — Events field on Engine, emits PipelineStarted/Completed/Failed, StageStarted/Completed/Failed/Retrying, CheckpointSaved

Tests: 6 test functions in events_test.go + `TestEventsEmitted` in engine_test.go

### 4.4 Context logs (append-only) — COMPLETED

**Files modified:**
- `internal/attractor/runtime/context.go` — Added `logs []string` field, `AppendLog()`, `Logs()` methods; updated `Clone()` to include logs
- `internal/attractor/runtime/checkpoint.go` — Added `Logs []string` field to Checkpoint
- `internal/attractor/engine.go` — Checkpoint save/resume includes logs

Tests: 3 test functions in context_log_test.go + 2 in checkpoint_test.go + `TestContextLogs` in engine_test.go

### 4.5 Artifact store — COMPLETED

**Files created:**
- `internal/attractor/artifact/store.go` — Store with NewStore, Store/Retrieve/Has/List/Remove/Clear. 100KB file-backing threshold.

**Files modified:**
- `internal/attractor/attractor.go` — Added `Artifacts *artifact.Store` to EngineConfig
- `internal/attractor/engine.go` — Added `Artifacts` field, auto-created in initialize from RunDir artifacts dir

Tests: 9 test functions in store_test.go + `TestArtifactStoreAvailable` in engine_test.go

### 4.6 Fidelity runtime behavior — COMPLETED

**Files created:**
- `internal/attractor/fidelity/preamble.go` — `GeneratePreamble()` with mode-specific summaries: truncate (goal+runID), compact (bullet summary), summary:lo/med/hi (token-budgeted), full (empty)

**Files modified:**
- `internal/attractor/engine.go` — `executeNode()` now generates preamble and sets `internal.preamble` context key

Tests: 6 test functions in preamble_test.go + `TestPreambleSetInContext` in engine_test.go

### 4.7 Duration value type parsing — COMPLETED

**Files created:**
- `internal/attractor/model/duration.go` — `ParseDuration()` function, `DurationAttr()` on Node and Graph

**Files modified:**
- `internal/attractor/handler/manager_loop.go` — Updated to use `node.DurationAttr()` instead of local `parseDuration()`; local wrapper now delegates to `model.ParseDuration()`

Tests: 3 test functions in duration_test.go (ParseDuration with 10 subtests, NodeDurationAttr, GraphDurationAttr)

---

## Phase 5: Agent Improvements — COMPLETED

All 6 items implemented and tested (27+ new test functions across 4 files, all passing).

### 5.1 Fix truncation order (already in Phase 1.6)

### 5.2 Environment context block — COMPLETED

**File modified:** `internal/agent/prompt/builder.go`

Replaced the single-line `Working directory:` with a full `<environment>` XML block per spec:

```xml
<environment>
Working directory: /path/to/project
Is git repository: true
Git branch: main
Platform: linux
OS version: 6.18.13-arch1-1
Today's date: 2026-03-03
Model: claude-sonnet-4-20250514
</environment>
```

Added `WithModel(model)` and `WithPlatformInfo()` builder methods. `buildEnvironment()` calls `git rev-parse` for repo/branch detection, `runtime.GOOS` for platform, `uname -r` for OS version. Wired into `session.go:buildRequest()` via `pb.WithModel(s.cfg.Model).WithPlatformInfo()`.

Tests: 4 new test functions in builder_test.go (environment block, model, empty model, platform info).

### 5.3 Project document discovery — COMPLETED

**File created:** `internal/agent/prompt/discovery.go`

Implemented `DiscoverProjectDocs(workDir, providerName)` that auto-discovers project instruction files:
- Walks from git root (via `git rev-parse --show-toplevel`) to workDir
- Looks for `AGENTS.md` (always) + provider-specific files: `CLAUDE.md` (anthropic), `GEMINI.md` (gemini), `.codex/instructions.md` (openai)
- Concatenates root-first, deeper files appended; truncates at 32KB
- Helper: `pathChain(root, target)` returns directory chain from root to target (inclusive)

Wired into `session.go:buildRequest()` — called when `cfg.ProjectDoc == ""`, so explicit docs take precedence over auto-discovery.

Tests: 10 new test functions in discovery_test.go (pathChain variants, no files, with AGENTS.md, provider-specific, wrong provider, truncation, multiple files, empty file).

### 5.4 Subagent tools — COMPLETED

**File created:** `internal/agent/tool/subagent.go`

Defined `SubagentSpawner` interface (in tool package to avoid circular deps) and `SubagentResult` struct. Implemented 4 tools:

| Tool | Params | Behavior |
|------|--------|----------|
| `spawn_agent` | `{task, agent_id}` | Spawns child agent via `mgr.Spawn()` |
| `send_input` | `{agent_id, message}` | Injects steering message via `mgr.SendInput()` |
| `wait` | `{agent_id}` | Blocks until child completes, returns formatted result |
| `close_agent` | `{agent_id}` | Cancels and cleans up child agent |

**File modified:** `internal/agent/session.go`

Added `RegisterSubagentTools(mgr)` method on Session with `subagentAdapter` that bridges `SubagentManager` → `SubagentSpawner`. Registers all 4 tools into the session's registry.

Tests: 17 new test functions in subagent_test.go (all tools with success/error/missing-args cases + tool metadata test).

### 5.5 Follow-up queue — COMPLETED

**File modified:** `internal/agent/session.go`

Added `followup []message.Message` field to Session. Added `FollowUp(content string)` method that queues a user message. At natural exit (no tool calls), the session now checks the followup queue: if non-empty, dequeues the next message, appends it to history, emits `turn.end` with `exit: "followup"`, and continues the loop instead of returning.

Tests: 3 new test functions in session_test.go (single followup, multiple followups, followup events).

### 5.6 Missing Config options — COMPLETED

**File modified:** `internal/agent/config.go`

Added 3 new fields to Config:
- `ReasoningEffort string` — passed through to `llm.Request.ReasoningEffort` in `buildRequest()`
- `EnableLoopDetection *bool` (default true via `loopEnabled()`) — gates the `detector.Record()` call in the tool execution loop
- `LoopDetectionWindow int` (default 10 via `loopWindow()`) — passed to `loop.NewDetector()` in `NewSession()`

`Streaming bool` was already added as part of Phase 6.2.

---

## Phase 6: LLM Client Features — COMPLETED

All 4 items implemented and tested (40+ new tests across provider and client packages, all passing).

### 6.1 `Client.FromEnv()` — COMPLETED

> Implemented as `clientFromEnv()` in `cmd/dfgo/main.go` rather than in `internal/llm/client.go`, because `llm` cannot import `provider` (circular dependency). The CLI is the natural wiring point. The `ConfigurationError` type was added to `internal/llm/errors.go` as planned.

**Files modified:**
- `internal/llm/errors.go` — Added `ConfigurationError` type
- `cmd/dfgo/main.go` — Added `clientFromEnv(verbose)` that auto-discovers `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`; creates provider adapters; attaches retry + logging middleware. Wires `DefaultAgentSessionFactory(client, env)` into `EngineConfig`. Graceful fallback: if no keys set, coding_agent nodes use the stub handler.
- `internal/attractor/handler/coding_agent.go` — `DefaultAgentSessionFactory` now reads `stream` node attribute into `Config.Streaming`

The full end-to-end path is now functional:
```
dfgo run pipeline.dot  (with ANTHROPIC_API_KEY set)
  → clientFromEnv() → Client with Anthropic + retry + logging
  → DefaultAgentSessionFactory(client, env) → EngineConfig
  → coding_agent node → agent.Session.Run() → real LLM calls
```

### 6.2 Streaming support — COMPLETED

> Implemented with a scanner-pattern API instead of the channel-based design originally proposed here. See `docs/design/llm-client.md` (Streaming section) and `docs/prompts/plan-streaming.md` for full details.

**Files created:**
- `internal/llm/stream.go` — `Stream` type (scanner pattern: `Next()`/`Event()`/`Err()`/`Response()`), `StreamEvent` with 6 event types, `CompleteToStream()` fallback
- `internal/llm/sse.go` — Shared W3C-compliant SSE parser used by all three providers
- `internal/llm/provider/anthropic_stream.go` — Anthropic `CompleteStream()`
- `internal/llm/provider/openai_stream.go` — OpenAI Responses API `CompleteStream()`
- `internal/llm/provider/gemini_stream.go` — Gemini `CompleteStream()`

**Files modified:**
- `internal/llm/provider.go` — Added optional `StreamingProvider` interface (backward compatible)
- `internal/llm/client.go` — Added `Client.Stream()` with type-assert routing + fallback
- `internal/llm/middleware.go` — Both `loggingAdapter` and `retryAdapter` implement `CompleteStream()`
- `internal/agent/config.go` — Added `Streaming bool` field
- `internal/agent/event/event.go` — Added `llm.stream.start`, `llm.chunk`, `llm.stream.end` events
- `internal/agent/session.go` — Added `streamTurn()` method, streaming branch in `Run()`

Tests: 33 new tests across 7 files (SSE parser, stream type, all 3 providers, client routing, agent session streaming). All pass.

### 6.3 Anthropic prompt caching — COMPLETED

**File modified:** `internal/llm/provider/anthropic.go`

Added `injectCacheBreakpoints(ar)` method called at end of `buildRequest()`. Auto-injects `cache_control: {"type": "ephemeral"}` on three breakpoints:
1. **System prompt** — last text block in the system array
2. **Tool definitions** — last tool in the tools array
3. **Last user message** — last content block of the last user-role message

Added `CacheControl *anthropicCacheControl` field to both `anthropicContentBlock` and `anthropicTool` structs. Both `Complete()` and `CompleteStream()` now send the `anthropic-beta: prompt-caching-2024-07-31` header on every request.

Tests: 2 new test functions in provider_test.go (cache breakpoint placement, no-panic on empty request).

### 6.4 Beta headers support — COMPLETED

**File modified:** `internal/llm/provider/anthropic.go`

Added `betaHeaders(req)` method that always includes `prompt-caching-2024-07-31` and merges any user-specified headers from `req.ProviderOptions["anthropic"]["beta_headers"]` (supports both string and `[]any` formats). Both `Complete()` and `CompleteStream()` call `betaHeaders()` to set the `anthropic-beta` HTTP header.

Tests: 5 new test functions in provider_test.go (default headers, merge string, merge list, empty options, end-to-end HTTP header verification).

---

## Phase 7: Nice-to-Haves (Lower Priority)

These are not blocking correctness or core features. Implement as time allows.

### 7.1 HTTP server mode

New package `internal/attractor/server/` with net/http handlers. 9 endpoints per spec. SSE for `/events`. Uses Engine internally with async goroutine. Not required for CLI usage.

### 7.2 Model catalog

New file `internal/llm/catalog.go`. Static registry of known models with context window, capabilities, costs. `GetModelInfo()`, `ListModels()`, `GetLatestModel()`.

### 7.3 OpenAI-compatible adapter

New file `internal/llm/provider/openai_compat.go`. Chat Completions endpoint (`/v1/chat/completions`) for vLLM, Ollama, etc. Subset of OpenAI adapter without Responses API features.

### 7.4 CallbackInterviewer

New file `internal/attractor/interviewer/callback.go`. Delegates Ask() to a user-provided `func(Question) (Answer, error)`.

### 7.5 Gemini-specific tools

Add `read_many_files`, `list_dir` tools. Optional `web_search`, `web_fetch` tools for Gemini profile.

---

## Verification

After each phase, run:
```bash
go build ./...
go test ./...
```

### Phase-specific testing:

- **Phase 1:** ~~Add tests for retry backoff (verify delays), goal gate chain resolution, failure routing, truncation order fix.~~ DONE (30+ new tests, all passing, 77.5% coverage)
- **Phase 2:** ~~Add test cases for each new validation rule.~~ DONE (6 new test functions, 14 test cases, all passing). Also added `TestParseStylesheetErrors` in style package.
- **Phase 3:** ~~Add tests for fan-in ranking, parallel error_policy/max_parallel, manager loop cycles.~~ DONE (18 new tests, all passing). Run `go test ./internal/attractor/handler/...`
- **Phase 4:** ~~Add tests for stylesheet application, status.json writing, artifact store CRUD, preamble generation.~~ DONE (40+ new tests across 8 files, all passing). Run `go test ./internal/attractor/...`
- **Phase 5:** ~~Add tests for env context block, project doc discovery, subagent tools, follow-up queue.~~ DONE (46+ new tests across 4 files, all passing). Run `go test ./internal/agent/...`
- **Phase 6:** ~~Streaming tests~~ DONE (33 tests). ~~FromEnv, cache injection~~ DONE (7 new tests). Run `go test ./internal/llm/...`

### Integration test:
~~Update `testdata/pipelines/retry.dot` to exercise the backoff + goal gate + retry_target chain. Add a new `testdata/pipelines/full_features.dot` that exercises stylesheet, fidelity, parallel with error_policy, and fan-in ranking.~~ DONE (8 new integration tests + 3 helper handlers in engine_test.go, 2 updated/new DOT fixtures, all passing). Run `go test ./internal/attractor/ -run 'TestDefaultMaxRetry|TestEdgeSelection|TestFidelityModeTruncate|TestEventSequencing|TestParallelFanIn|TestRetryDotFull|TestFullFeatures'`
