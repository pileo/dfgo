// Package profile defines provider-aligned tool profiles for the coding agent.
// Each profile specifies which tools and system prompt style to use
// based on the underlying LLM provider's capabilities.
package profile

import "dfgo/internal/agent/tool"

// Profile describes a provider-specific configuration for the agent.
type Profile interface {
	// Name returns the profile identifier (e.g., "anthropic", "openai", "gemini").
	Name() string
	// CoreTools returns the names of tools to register for this provider.
	CoreTools() []string
	// EditTool returns the preferred edit tool name ("edit_file" or "apply_patch").
	EditTool() string
	// SystemPrompt returns the base system prompt template for this provider.
	SystemPrompt() string
	// ContextWindowSize returns the provider's typical context window in tokens.
	ContextWindowSize() int
}

// ConfigureRegistry returns a registry with tools matching the given profile.
// It starts from the default registry and filters to only the profile's tools.
func ConfigureRegistry(p Profile) *tool.Registry {
	all := tool.DefaultRegistry()
	r := tool.NewRegistry()
	for _, name := range p.CoreTools() {
		if t, ok := all.Lookup(name); ok {
			r.Register(t)
		}
	}
	return r
}
