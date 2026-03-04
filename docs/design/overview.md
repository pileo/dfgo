# Architecture Overview

dfgo is a pipeline orchestration engine where pipelines are declared as Graphviz DOT digraphs. Nodes are stages (LLM calls, human approvals, external tools) and edges define conditional transitions between them.

## Core Abstractions

```
DOT source → Parser → Graph → Validator → Engine → Outcome
                                             ↑
                                    Handler Registry
                                    Edge Selector
                                    Checkpoint
                                    Interviewer
                                    Transforms
```

### Immutable vs Mutable

The system separates immutable graph data from mutable execution state:

- **Immutable** (`model/`): `Graph`, `Node`, `Edge` — parsed once, never modified during execution
- **Mutable** (`runtime/`): `Context`, `Outcome`, `Checkpoint` — change as the pipeline runs

### Engine Lifecycle

The engine runs a 5-phase lifecycle for every pipeline:

```
Parse → Validate → Initialize → Execute → Finalize
```

1. **Parse**: DOT source → `model.Graph` via hand-rolled lexer/parser
2. **Validate**: 13 lint rules check structural correctness (start/exit node constraints, edge targets, condition syntax, stylesheet syntax, fidelity modes, retry targets, known types, etc.)
3. **Initialize**: Generate run ID, create run directory, create event emitter + artifact store, load checkpoint if resuming, seed pipeline context
4. **Execute**: Loop through nodes — for each node: emit events, generate fidelity preamble, execute handler, write status.json, apply context updates, select next edge, advance. Handles retries and goal gates.
5. **Finalize**: Write final checkpoint, emit completion event, update manifest

### Key Design Decisions

1. **`map[string]string` attributes everywhere** — nodes, edges, and graph-level attributes are all string maps. Type coercion (`IntAttr`, `BoolAttr`, `FloatAttr`, `DurationAttr`) happens lazily at point of use. This matches DOT's string-native format and keeps the model simple.

2. **`Order` field on Node and Edge** — preserves DOT declaration order for deterministic traversal and reproducible test output.

3. **Custom DOT parser** — hand-rolled lexer + recursive-descent parser (~300 lines) instead of a library. The DOT subset we need is small; owning the parser gives better error messages and avoids a heavy dependency.

4. **Handler registry with two lookup tiers** — `type` attribute takes priority over `shape` attribute. This lets a node declare `type="codergen"` explicitly, or fall back to shape-based dispatch (e.g., `shape="Mdiamond"` → start handler).

5. **Optional handler interfaces** — `FidelityAwareHandler` and `SingleExecutionHandler` are opt-in capability markers. Handlers implement them only if they need fidelity control or single-execution semantics.

6. **Failure classification** — outcomes carry a `FailureClass` (transient, deterministic, canceled, budget-exhausted) so the engine can make informed retry/escalation decisions.

7. **Atomic checkpoint writes** — write to temp file + rename to prevent corruption if the process crashes mid-write.

## Package Dependency Graph

```
cmd/dfgo (cobra root + run/serve subcommands)
  ├─ server (HTTP API server, stdlib net/http only)
  │    ├─ runmgr (run lifecycle tracking)
  │    └─ sse (fan-out broadcaster with replay)
  ├─ attractor (engine, RunPipeline facade)
  │    ├─ dot (lexer, parser)
  │    ├─ model (Graph, Node, Edge)
  │    ├─ runtime (Context, Outcome, Checkpoint)
  │    ├─ validate (lint rules)
  │    ├─ cond (condition parser/evaluator)
  │    ├─ edge (edge selector)
  │    ├─ handler (Handler interface, all handlers)
  │    │    ├─ interviewer (incl. HTTP interviewer for server)
  │    │    ├─ fidelity
  │    │    ├─ agent ←────────── coding_agent handler
  │    │    └─ llm ←──────────── LLMCodergenBackend
  │    ├─ style (stylesheet with class support + Apply transform)
  │    ├─ events (pipeline observability events + emitter)
  │    ├─ artifact (artifact store: in-memory + file-backed)
  │    ├─ transform (variable expansion)
  │    └─ rundir (run directory, manifest)
  ├─ llm (unified LLM client)
  │    └─ provider (anthropic, openai, gemini)
  └─ agent (coding agent session, core loop)
       ├─ message (agent-level message types)
       ├─ event (async event emitter)
       ├─ tool (Tool interface, registry, 7 core tools)
       │    └─ truncate (output truncation)
       ├─ loop (loop detection)
       ├─ execenv (execution environment)
       ├─ profile (provider profiles)
       └─ prompt (system prompt builder)
```

All packages under `internal/` — not importable by external code.

## External Dependencies

| Dependency | Used by | Purpose |
|---|---|---|
| `github.com/google/uuid` | engine | Generate unique run IDs |
| `github.com/spf13/cobra` | CLI | Subcommand and flag parsing |
| `golang.org/x/sync/errgroup` | (reserved) | Available for future use (parallel handler uses `sync.WaitGroup` + channel semaphore) |

Everything else is Go stdlib: `log/slog`, `encoding/json`, `os/exec`, `sync`, `context`, `regexp`, `strconv`, `time`.

## Testing

Tests use `go test ./...` across all packages. Key layers:

- **Unit tests**: each package has its own `*_test.go` files testing handlers, validators, edge selection, fidelity, parsing, etc.
- **Integration tests**: `internal/attractor/engine_test.go` tests feature composition through the full engine lifecycle using DOT fixtures from `testdata/pipelines/` and inline DOT graphs with custom handler stubs.
- **Server integration tests**: `internal/server/server_test.go` tests the full HTTP API via `httptest.Server` — submit, status, SSE streaming, context, cancel, human gate question/answer flow.
- **DOT fixtures**: `testdata/pipelines/` contains 8 valid and 3 invalid pipelines ranging from minimal (`simple.dot`) to comprehensive (`retry.dot` exercises retry/goal-gate/allow_partial composition; `full_features.dot` exercises stylesheet + fidelity + parallel + fan-in + review loops).

See [engine.md](engine.md#test-pipelines) for the full fixture catalog and integration test matrix.
See [http-server.md](http-server.md) for the HTTP API documentation.
