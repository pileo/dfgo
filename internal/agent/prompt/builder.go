// Package prompt builds layered system prompts for the coding agent.
// Prompts are assembled from 5 layers: provider base → env context →
// tool descriptions → project docs → user overrides.
package prompt

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

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
	model      string
	platform   bool // whether to include platform info
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

// WithModel sets the model identifier for the environment block.
func (b *Builder) WithModel(model string) *Builder {
	b.model = model
	return b
}

// WithPlatformInfo enables full platform info in the environment block.
func (b *Builder) WithPlatformInfo() *Builder {
	b.platform = true
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
	sections = append(sections, b.buildEnvironment())

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

// buildEnvironment constructs the <environment> XML block for Layer 2.
func (b *Builder) buildEnvironment() string {
	var buf strings.Builder
	buf.WriteString("<environment>\n")
	buf.WriteString(fmt.Sprintf("Working directory: %s\n", b.workDir))

	// Git info.
	isGit := gitIsRepo(b.workDir)
	buf.WriteString(fmt.Sprintf("Is git repository: %t\n", isGit))
	if isGit {
		if branch := gitBranch(b.workDir); branch != "" {
			buf.WriteString(fmt.Sprintf("Git branch: %s\n", branch))
		}
	}

	if b.platform {
		buf.WriteString(fmt.Sprintf("Platform: %s\n", runtime.GOOS))
		if ver := osVersion(); ver != "" {
			buf.WriteString(fmt.Sprintf("OS version: %s\n", ver))
		}
	}

	buf.WriteString(fmt.Sprintf("Today's date: %s\n", time.Now().Format("2006-01-02")))

	if b.model != "" {
		buf.WriteString(fmt.Sprintf("Model: %s\n", b.model))
	}

	buf.WriteString("</environment>")
	return buf.String()
}

func gitIsRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func gitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func osVersion() string {
	cmd := exec.Command("uname", "-r")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
