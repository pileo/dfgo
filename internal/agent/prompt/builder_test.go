package prompt

import (
	"strings"
	"testing"

	"dfgo/internal/agent/profile"
	"dfgo/internal/agent/tool"
)

func TestBuilderBasic(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/tmp/work")

	result := b.Build()
	if !strings.Contains(result, "autonomous coding agent") {
		t.Error("missing base prompt")
	}
	if !strings.Contains(result, "/tmp/work") {
		t.Error("missing working directory")
	}
	if !strings.Contains(result, "read_file") {
		t.Error("missing tool descriptions")
	}
}

func TestBuilderWithProjectDoc(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/work").WithProjectDoc("This is a Go project")

	result := b.Build()
	if !strings.Contains(result, "This is a Go project") {
		t.Error("missing project doc")
	}
}

func TestBuilderProjectDocTruncation(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	longDoc := strings.Repeat("x", 40000)
	b := NewBuilder(p, r, "/work").WithProjectDoc(longDoc)

	result := b.Build()
	if !strings.Contains(result, "... [truncated]") {
		t.Error("expected truncation marker")
	}
}

func TestBuilderWithUserPrompt(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/work").WithUserPrompt("Always use tabs")

	result := b.Build()
	if !strings.Contains(result, "Always use tabs") {
		t.Error("missing user prompt")
	}
}

func TestBuilderAllLayers(t *testing.T) {
	p := profile.OpenAI{}
	r := profile.ConfigureRegistry(p)
	b := NewBuilder(p, r, "/project").
		WithProjectDoc("Project README").
		WithUserPrompt("Custom instructions")

	result := b.Build()
	// Verify all 5 layers present.
	if !strings.Contains(result, "autonomous coding agent") {
		t.Error("layer 1: missing base prompt")
	}
	if !strings.Contains(result, "/project") {
		t.Error("layer 2: missing env context")
	}
	if !strings.Contains(result, "apply_patch") {
		t.Error("layer 3: missing tool descriptions")
	}
	if !strings.Contains(result, "Project README") {
		t.Error("layer 4: missing project doc")
	}
	if !strings.Contains(result, "Custom instructions") {
		t.Error("layer 5: missing user prompt")
	}
}
