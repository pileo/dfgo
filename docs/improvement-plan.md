# dfgo Improvement Plan

Competitive analysis of dfgo against the Attractor spec, Kilroy metaspec, and 5 community implementations (Forge/Rust, brynary/TypeScript, anishkny/Python, attractor-ruby/Ruby, samueljklee/Python).

## Current State

**19,306 LOC | 121 Go files | ~80% test coverage | 3 providers | 10 handlers | 12 validation rules**

dfgo has a solid foundation: full pipeline lifecycle, DOT parsing, 5-step edge selection, thread-safe context, atomic checkpoints, a coding agent with 7+ tools, and clean abstractions throughout. It's a well-architected ~90% implementation of the core Attractor engine spec.

## dfgo's Existing Advantages

Areas where dfgo is already ahead of most community implementations:

1. **Parallel join policies** -- All 4 (wait_all, first_success, k_of_n, quorum) + 3 error policies. Most community impls have only wait_all.
2. **Retry presets** -- 5 backoff presets with jitter. Only brynary also has named presets.
3. **Fidelity system** -- 6 modes with resolution chain. No community impl has this.
4. **CSS-like style system** -- Specificity-based stylesheet. Unique feature.
5. **Agent loop detection** -- SHA-256 signature hashing with period 1/2/3 detection.
6. **5-step edge selection** -- Complete implementation with weight + lexical tiebreaking.
7. **Atomic checkpoints** -- Write-tmp-then-rename with full context serialization.
8. **No external LLM SDK deps** -- Direct net/http for all providers. Leaner than most.
9. **12 validation rules** -- Most comprehensive rule set (Forge has 13, but with a trait vs interface).
10. **Two-phase output truncation** -- Char limit first, then line limit. Preserves context better.

## Gap Analysis: dfgo vs Spec + Community

### Critical Gaps (Metaspec MUST requirements not met)

| Gap | Spec Requirement | dfgo Status | Community Coverage |
|-----|-----------------|-------------|-------------------|
| **Git-first execution** | Per-run branch, one commit per node, worktree isolation | Not implemented | 0/5 have it |
| **CXDB integration** | Context per run, typed turns, blob storage, triple resume | Not implemented | 0/5 have it |
| **CLI backend** | Subprocess execution of `claude`/`codex`/`gemini` CLIs | Not implemented | 0/5 have it |
| **Missing providers** | 7+ providers (OpenAI, Anthropic, Google, Kimi, ZAI, Cerebras, Minimax) | Only 3 providers | 0/5 have all |
| **OpenRouter model catalog** | Per-run snapshot, pinned vs on_run_start policies | Not using OpenRouter | 0/5 have it |
| **Run configuration YAML** | Full schema with per-provider backend selection, setup commands | CLI args only | 0/5 have it |
| **Repo cleanliness check** | MUST fail if uncommitted changes (unless `--allow-dirty`) | Not implemented | 0/5 have it |
| **`final.json` output** | Machine-readable run result with CXDB context ID | Not implemented | 0/5 have it |
| **Missing validation rules** | 20+ lint rules required | Only 12 of 20+ | Varies |
| **Restart artifacts** | Base logs root + `restart-<n>/` subdirectories | Not implemented | 0/5 have it |

### High-Value Gaps (Community consensus + spec alignment)

| Gap | Community Signal | dfgo Status | Effort |
|-----|-----------------|-------------|--------|
| **HTTP Server + SSE** | 5/5 implementations | Not implemented | Medium |
| **Simulation/test backend** | 3/5 implementations (Ruby best) | Not implemented | Small |
| **Manager loop handler** | 4/5 implementations | Stub only | Medium-Large |
| **Tool call hooks (pre/post)** | 2/5 but high impact | Not implemented | Small-Medium |
| **Model capability catalog** | 4/5 implementations | Basic catalog exists, needs structured metadata | Small |
| **LLM middleware chain** | 3/5 implementations | Has logging+retry, needs composable onion model | Medium |
| **Steering injection** | 3/5 implementations | Not implemented | Small-Medium |
| **Recording/Queue interviewers** | 3/5 implementations | Stub only | Small |

### Nice-to-Have Gaps (Community innovations, lower priority)

| Gap | Source | dfgo Status | Effort |
|-----|--------|-------------|--------|
| **Graph composition/merge** | brynary only | Not implemented | Medium |
| **Named retry presets** | brynary only (dfgo has unnamed presets) | Partial | Small |
| **Structured LLM output** | brynary only | Not implemented | Medium |
| **Typed DOT attributes** | Forge only | Uses `map[string]string` everywhere | Medium |
| **Exec environment wrappers** | 3/5 (EnvFilter, LoggingEnv, ReadonlyEnv) | Basic LocalExecutionEnvironment | Small-Medium |
| **Parallel join policies** | 2/5 implementations | Has all 4 policies -- done | Done |
| **Pipeline-as-library API** | 4/5 implementations | Has `RunPipeline` facade -- mostly done | Mostly done |

## Improvement Plan

### Phase 1: Spec Compliance Foundation

These close the gap on metaspec MUST requirements that no community implementation has either -- making dfgo the first fully compliant implementation.

#### 1.1 Run Configuration Schema
- YAML/JSON config with per-provider backend selection (`api` vs `cli`)
- Setup commands with timeout
- Push remote config
- `--allow-dirty` flag
- Effort: Small-Medium

#### 1.2 Git-First Execution
- Create `attractor/run/<run_id>` branch from clean HEAD
- One commit per executed node (even empty commits)
- Record commit SHA in checkpoint
- Worktree isolation for parallel branches and CLI backends
- Fail-fast if not a git repo or repo is dirty
- Restart artifacts: base logs root + `restart-<n>/` subdirectories
- Run-scoped scratch: `.ai/runs/$KILROY_RUN_ID/`
- Effort: Large

#### 1.3 CXDB Integration
- One CXDB context per pipeline run
- Fork contexts for parallel branches
- 10+ typed turns (pipeline.started, stage.completed, etc.)
- Blob storage for large artifacts
- `{logs_root}` recorded in CXDB
- Effort: Large

#### 1.4 Additional Providers
- Kimi: `anthropic_messages` protocol family adapter
- ZAI: `openai_chat_completions` protocol family adapter
- Cerebras: `openai_chat_completions` protocol family adapter
- Minimax: protocol family TBD
- All are thin wrappers over existing adapters with different base URLs
- Effort: Small-Medium

#### 1.5 OpenRouter Model Catalog
- Fetch from OpenRouter API at run start
- Per-run snapshot (freeze for run lifetime including resume)
- `pinned` vs `on_run_start` update policies
- Repo-pinned JSON snapshot option
- Effort: Medium

#### 1.6 Missing Validation Rules
- `goal_gate_exit_status_contract` (ERROR)
- `prompt_file_conflict` (ERROR)
- `llm_provider_required` (ERROR)
- `goal_gate_prompt_status_hint` (WARNING)
- `prompt_on_conditional_node` (WARNING)
- `loop_restart_failure_class_guard` (WARNING)
- `escalation_models_syntax` (WARNING)
- Plus any others from the full spec
- Effort: Small

### Phase 2: Community Consensus Features

Features that 3-5 community implementations independently built, representing proven demand.

#### 2.1 HTTP Server + SSE Event Streaming
- POST `/run` to submit pipeline
- GET `/status/:id` for run status
- GET `/events/:id` for SSE stream
- POST `/cancel/:id` for cancellation
- POST `/answer/:id` for human gate responses
- GET `/context/:id` for context inspection
- Effort: Medium

#### 2.2 Simulation Backend for Testing
- Node-specific response maps
- Regex pattern matching for prompts
- Callback-based dynamic behavior (Ruby's approach)
- Deterministic CI testing without LLM calls
- Effort: Small

#### 2.3 Preflight / Dry-Run Mode
- `--preflight` / `--test-run` flag
- Validates everything (parse, lint, config, git, CXDB) without executing
- Pairs well with simulation backend
- Effort: Small

#### 2.4 Manager Loop Handler (Full Implementation) ✅
- ~~Wire up child engine integration (currently stub)~~ Done: `ChildEngine` wired in `executeNode()`, children executed sequentially with context merging
- ~~Poll/steer/wait cycle with configurable intervals~~ Done: observe/wait actions, `manager.poll_interval`
- ~~Stop condition evaluation~~ Done: `manager.stop_condition` via `cond.Eval`
- ~~Child telemetry ingestion~~ Done: `stack.child.status` observed each cycle
- Integration test: `TestManagerLoopIntegration` + 8 unit tests
- Effort: Medium-Large

#### 2.5 Tool Call Hooks (Pre/Post)
- Configurable shell scripts before/after each agent tool call
- Pre-hooks can veto (non-zero exit = skip)
- Enable security scanning, compliance checks, custom logging
- Effort: Small-Medium

#### 2.6 Steering Injection
- `session.Steer("guidance")` queued for post-tool-round injection
- `session.FollowUp("msg")` queued for post-input processing
- SteeringTurn in message history converted to user-role messages for LLM
- Effort: Small-Medium

### Phase 3: CLI Backend + Triple Resume

Production robustness features.

#### 3.1 CLI Backend Adapters
- Anthropic: `claude --print` / `claude -p` with `text|json|stream-json`
- OpenAI: `codex exec --json` for JSONL events
- Google: `gemini --prompt` with `--yolo`/`--approval-mode auto_edit`
- Capture enough for replay: executable, argv, env allowlist, cwd
- Non-interactive execution only
- Effort: Large

#### 3.2 Triple Resume
- Resume from `checkpoint.json` (filesystem)
- Resume from CXDB context head (trajectory state)
- Resume from git branch commit chain (code state)
- On resume: reset worktree to last checkpoint SHA
- Fidelity downgrade rule on resume
- Effort: Medium-Large (builds on 1.2 + 1.3)

#### 3.3 Failure Resilience Features
- Failure signature normalization (hash-based dedup across restarts)
- Deterministic failure cycle breaker (kill on repeated identical failures)
- Stuck-cycle breaker (max node visits per iteration)
- Stall watchdog (kill runs with no progress)
- Effort: Medium

### Phase 4: Polish & Differentiation

#### 4.1 Recording & Queue Interviewers
- Flesh out existing stubs
- Effort: Small

#### 4.2 Composable LLM Middleware Chain
- Onion model for request/response processing
- Standard middleware: logging, retry, cost tracking, caching, prompt injection detection
- Effort: Medium

#### 4.3 Execution Environment Wrappers
- EnvFilter (strip secrets from environment)
- LoggingEnv (audit trails)
- ReadonlyEnv (safe evaluation)
- Effort: Small-Medium

#### 4.4 Enhanced Model Capability Catalog
- Structured metadata: context_window, max_output_tokens, supports_tools, supports_vision, supports_thinking
- Effort: Small

#### 4.5 Detached Execution
- `--detach` flag with `setsid` + PID tracking
- PID file + log output
- Effort: Small

#### 4.6 Graph Composition / Merge Transform
- Import/merge sub-graphs from separate DOT files
- Namespace-prefixed node IDs
- Effort: Medium

## Strategic Summary

| Phase | Theme | Makes dfgo... |
|-------|-------|---------------|
| **1** | Spec Compliance | First fully compliant Attractor implementation |
| **2** | Community Consensus | Feature-competitive with best community impls |
| **3** | Production Robustness | The most production-ready implementation |
| **4** | Polish | Best-in-class developer experience |

No community implementation has tackled the hard problems (git-first, CXDB, CLI backend, triple resume). dfgo already has the strongest engine foundation. Phases 1-3 would make it the only implementation that's both spec-compliant AND feature-rich.

## Source Documents

- [Community Implementations Analysis](https://github.com/danshapiro/kilroy/blob/main/docs/community-implementations-analysis.md)
- [Attractor Spec (README)](https://github.com/danshapiro/kilroy/blob/main/docs/strongdm/attractor/README.md)
- [Kilroy Metaspec](https://github.com/danshapiro/kilroy/blob/main/docs/strongdm/attractor/kilroy-metaspec.md)
