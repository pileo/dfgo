# Subagents

**Package**: `internal/agent` (in `subagent.go`)

Subagents are independent child sessions spawned from a parent agent. Each has its own message history and tool loop but shares the execution environment with the parent.

## SubagentManager

```go
mgr := agent.NewSubagentManager(parentConfig, 0)  // depth=0 for top-level
defer mgr.CloseAll()

// Spawn a child agent.
err := mgr.Spawn(ctx, "refactor-auth", "Refactor the auth module to use JWT")

// Optionally steer the child.
mgr.SendInput("refactor-auth", "Focus on the token validation first")

// Wait for completion.
result, err := mgr.Wait("refactor-auth")
fmt.Println(result.FinalText)

// Clean up.
mgr.Close("refactor-auth")
```

## Depth Limiting

Subagents are depth-limited to prevent unbounded recursion. The default maximum depth is 3. Attempting to spawn at or beyond the limit returns an error.

```go
mgr := agent.NewSubagentManager(cfg, 3)  // already at max
err := mgr.Spawn(ctx, "deep", "work")    // error: maximum depth exceeded
```

## Lifecycle

- **Spawn**: creates a new `Session` with the parent's config, starts `session.Run()` in a goroutine
- **SendInput**: injects a steering message into the child's next turn via `session.Steer()`
- **Wait**: blocks until the child completes, returns its `Result`
- **Close**: cancels the child's context and removes it from the manager
- **CloseAll**: cancels and removes all children

Each subagent ID must be unique within a manager. Attempting to spawn with a duplicate ID returns an error.
