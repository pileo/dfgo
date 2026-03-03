# dfgo

Attractor pipeline orchestration engine. Pipelines are declared as Graphviz DOT graphs where nodes are stages (LLM calls, human approvals, tools) and edges define conditional transitions.

## Prerequisites

- [Go 1.23+](https://go.dev/dl/)

## Building

```sh
# Compile all packages (binary goes to $GOPATH/bin or ./dfgo)
go build ./...

# Build just the CLI binary
go build -o dfgo ./cmd/dfgo
```

## Running

```sh
# Run a pipeline from a DOT file (auto-approve all human prompts)
go run ./cmd/dfgo run testdata/pipelines/linear.dot --auto-approve

# Or use the compiled binary
./dfgo run testdata/pipelines/linear.dot --auto-approve
```

### CLI flags

Flags belong to the `run` subcommand and can appear in any position:

| Flag | Default | Description |
|---|---|---|
| `--auto-approve` | `false` | Auto-approve all human interaction prompts |
| `--logs-dir` | `runs` | Directory for run logs and checkpoints |
| `--resume` | | Resume a previous run by its ID |
| `--verbose` | `false` | Enable debug-level logging |

### Example pipelines

```sh
go run ./cmd/dfgo run testdata/pipelines/simple.dot --auto-approve      # start → exit
go run ./cmd/dfgo run testdata/pipelines/linear.dot --auto-approve      # start → A → B → exit
go run ./cmd/dfgo run testdata/pipelines/branching.dot --auto-approve   # conditional routing
go run ./cmd/dfgo run testdata/pipelines/parallel.dot --auto-approve    # fan-out/fan-in
go run ./cmd/dfgo run testdata/pipelines/retry.dot --auto-approve       # goal gates + retries
```

## Testing

```sh
# Run all tests across every package
go test ./...

# Run tests for a single package
go test ./internal/attractor/dot/

# Run tests with verbose output (shows each test name and status)
go test ./... -v

# Run a specific test by name (-run takes a regex)
go test ./internal/attractor/ -run TestRunLinearPipeline -v

# Run tests with the race detector (catches concurrency bugs)
go test -race ./...

# Run tests and show coverage percentage per package
go test -cover ./...

# Generate an HTML coverage report you can open in a browser
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Code quality checks

```sh
# Static analysis — catches common bugs (unused vars, bad printf args, etc.)
go vet ./...

# Format all source files in-place (Go's canonical style, no config needed)
gofmt -w .
```

## Dependency management

```sh
# Download dependencies and update go.sum (run after editing go.mod or adding imports)
go mod tidy

# See the current dependency tree
go mod graph
```

## Project structure

```
cmd/dfgo/                  CLI entry point (cobra root + run subcommand)
internal/attractor/
  attractor.go             RunPipeline facade + EngineConfig
  engine.go                5-phase lifecycle, execution loop, retry
  model/graph.go           Graph, Node, Edge (immutable data)
  runtime/
    context.go             Thread-safe key-value pipeline context
    outcome.go             Stage result (SUCCESS, FAIL, RETRY, PARTIAL_SUCCESS)
    checkpoint.go          Atomic JSON checkpoint save/load
  dot/
    lexer.go               DOT tokenizer
    parser.go              Recursive descent DOT parser
  cond/cond.go             Condition expression parser + evaluator
  validate/
    validate.go            LintRule interface + runner
    rules.go               7 built-in validation rules
  edge/selector.go         5-step edge selection priority
  handler/
    handler.go             Handler interface + registry
    start.go               Start node (Mdiamond, no-op)
    exit.go                Exit node (Msquare, no-op)
    codergen.go            LLM code generation (CodergenBackend interface)
    coding_agent.go        Autonomous agent session (with event logging)
    wait_human.go          Human approval (Interviewer interface)
    conditional.go         Diamond routing (pass-through)
    parallel.go            Fan-out with join policies
    fan_in.go              Fan-in consolidation
    tool.go                External tool execution (os/exec)
    manager_loop.go        Iterative refinement loop
  interviewer/             Human interaction implementations
  style/stylesheet.go      CSS-like model stylesheet
  fidelity/fidelity.go     LLM reasoning effort modes
  rundir/rundir.go         Run directory + manifest
  transform/transform.go   Variable expansion ($goal, ${var})
internal/llm/              Unified LLM client (types, errors, middleware)
  provider/                Provider adapters (Anthropic, OpenAI, Gemini)
internal/agent/            Coding agent session + core loop
  message/                 Agent-level message types
  event/                   Async event emitter (14 typed events)
  tool/                    Tool interface + 7 core tools (read/write/edit/patch/shell/grep/glob)
    truncate/              Two-phase middle-cut output truncation
  loop/                    Loop detection (SHA-256 signature hashing)
  execenv/                 Execution environment (process group management)
  profile/                 Provider profiles (Anthropic/OpenAI/Gemini tool sets)
  prompt/                  5-layer system prompt builder
testdata/pipelines/        DOT fixture files
```
