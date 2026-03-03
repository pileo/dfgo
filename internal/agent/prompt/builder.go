// Package prompt builds layered system prompts for the coding agent.
// Prompts are assembled from 5 layers: provider base → env context →
// tool descriptions → project docs → user overrides.
package prompt

import (
	"fmt"
	"strings"

	"dfgo/internal/agent/profile"
	"dfgo/internal/agent/tool"
)

const maxProjectDocsBytes = 32 * 1024

// Builder constructs system prompts from multiple layers.
type Builder struct {
	profile    profile.Profile
	registry   *tool.Registry
	workDir    string
	projectDoc string
	userPrompt string
}

// NewBuilder creates a prompt builder for the given profile and tool registry.
func NewBuilder(p profile.Profile, r *tool.Registry, workDir string) *Builder {
	return &Builder{
		profile:  p,
		registry: r,
		workDir:  workDir,
	}
}

// WithProjectDoc adds project documentation (e.g., README, CLAUDE.md) to the prompt.
// Content is truncated to the 32KB budget if needed.
func (b *Builder) WithProjectDoc(doc string) *Builder {
	if len(doc) > maxProjectDocsBytes {
		doc = doc[:maxProjectDocsBytes] + "\n... [truncated]"
	}
	b.projectDoc = doc
	return b
}

// WithUserPrompt adds a user override section to the system prompt.
func (b *Builder) WithUserPrompt(prompt string) *Builder {
	b.userPrompt = prompt
	return b
}

// Build assembles the final system prompt from all layers.
func (b *Builder) Build() string {
	var sections []string

	// Layer 1: Provider base prompt.
	if base := b.profile.SystemPrompt(); base != "" {
		sections = append(sections, base)
	}

	// Layer 2: Environment context.
	envCtx := fmt.Sprintf("Working directory: %s", b.workDir)
	sections = append(sections, envCtx)

	// Layer 3: Tool descriptions.
	if tools := b.registry.All(); len(tools) > 0 {
		var toolSection strings.Builder
		toolSection.WriteString("Available tools:")
		for _, t := range tools {
			toolSection.WriteString(fmt.Sprintf("\n- %s: %s", t.Name(), t.Description()))
		}
		sections = append(sections, toolSection.String())
	}

	// Layer 4: Project documentation.
	if b.projectDoc != "" {
		sections = append(sections, "Project documentation:\n"+b.projectDoc)
	}

	// Layer 5: User overrides.
	if b.userPrompt != "" {
		sections = append(sections, "User instructions:\n"+b.userPrompt)
	}

	return strings.Join(sections, "\n\n")
}
