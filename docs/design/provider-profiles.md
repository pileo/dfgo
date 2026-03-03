# Provider Profiles

**Package**: `internal/agent/profile`

Profiles configure the agent's tool set and system prompt based on the underlying LLM provider's capabilities. Different providers work better with different editing tools and have different context window sizes.

## Interface

```go
type Profile interface {
    Name() string
    CoreTools() []string
    EditTool() string
    SystemPrompt() string
    ContextWindowSize() int
}
```

## Built-in Profiles

| Profile | Edit Tool | Context Window | Notes |
|---|---|---|---|
| `Anthropic` | `edit_file` | 200K tokens | String replacement editing |
| `OpenAI` | `apply_patch` | 128K tokens | Unified diff patches |
| `Gemini` | `edit_file` | 1M tokens | String replacement editing |

### Tool Selection

Each profile declares which tools from the default registry to include. This filters the tool registry at session creation time:

```go
reg := profile.ConfigureRegistry(profile.Anthropic{})
// Returns registry with: read_file, write_file, edit_file, shell, grep, glob
// (no apply_patch — Anthropic profile uses edit_file)

reg := profile.ConfigureRegistry(profile.OpenAI{})
// Returns registry with: read_file, write_file, apply_patch, shell, grep, glob
// (no edit_file — OpenAI profile uses apply_patch)
```

## System Prompt Builder

**Package**: `internal/agent/prompt`

Assembles the system prompt from 5 layers:

```
1. Provider base prompt    ← from profile.SystemPrompt()
2. Environment context     ← working directory
3. Tool descriptions       ← from registry.All()
4. Project documentation   ← optional, 32KB budget
5. User overrides          ← optional custom instructions
```

```go
builder := prompt.NewBuilder(profile, registry, "/path/to/project")
builder.WithProjectDoc(readmeContent)
builder.WithUserPrompt("Always run tests after changes")
systemPrompt := builder.Build()
```

Project documentation is truncated to 32KB if it exceeds the budget, with a `... [truncated]` marker appended.
