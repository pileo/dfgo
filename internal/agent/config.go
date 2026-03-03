// Package agent implements the coding agent loop.
package agent

import (
	"dfgo/internal/agent/execenv"
	"dfgo/internal/agent/profile"
	"dfgo/internal/llm"
)

// Config holds configuration for a Session.
type Config struct {
	// Client is the LLM client to use for completions.
	Client *llm.Client

	// Profile selects the provider-specific tool/prompt configuration.
	Profile profile.Profile

	// Env is the execution environment for tools.
	Env execenv.Environment

	// Model is the LLM model identifier (e.g., "claude-sonnet-4-20250514").
	Model string

	// MaxTurns limits the total number of agent turns (0 = unlimited).
	MaxTurns int

	// MaxRounds limits the number of LLM round-trips per turn (default 200).
	MaxRounds int

	// ProjectDoc is optional project documentation to include in the system prompt.
	ProjectDoc string

	// UserPrompt is optional user instructions appended to the system prompt.
	UserPrompt string

	// Temperature controls sampling (nil = provider default).
	Temperature *float64

	// MaxTokens limits the response length (nil = provider default).
	MaxTokens *int

	// Streaming enables streaming LLM responses (default: false).
	Streaming bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(client *llm.Client, env execenv.Environment) Config {
	return Config{
		Client:    client,
		Profile:   profile.Anthropic{},
		Env:       env,
		MaxRounds: 200,
	}
}
