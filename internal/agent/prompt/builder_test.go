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

func TestBuilderEnvironmentBlock(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/tmp/work")

	result := b.Build()

	// The environment block should be wrapped in XML tags.
	if !strings.Contains(result, "<environment>") {
		t.Error("missing <environment> opening tag")
	}
	if !strings.Contains(result, "</environment>") {
		t.Error("missing </environment> closing tag")
	}
	if !strings.Contains(result, "Working directory: /tmp/work") {
		t.Error("missing working directory in environment block")
	}
	if !strings.Contains(result, "Today's date:") {
		t.Error("missing today's date in environment block")
	}
	if !strings.Contains(result, "Is git repository:") {
		t.Error("missing git repository status in environment block")
	}
}

func TestBuilderWithModel(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/tmp/work").WithModel("test-model")

	result := b.Build()
	if !strings.Contains(result, "Model: test-model") {
		t.Error("missing model in environment block")
	}
}

func TestBuilderWithModelEmpty(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/tmp/work")

	result := b.Build()
	if strings.Contains(result, "Model:") {
		t.Error("model should not appear when not set")
	}
}

func TestBuilderWithPlatformInfo(t *testing.T) {
	p := profile.Anthropic{}
	r := tool.DefaultRegistry()
	b := NewBuilder(p, r, "/tmp/work").WithPlatformInfo()

	result := b.Build()
	if !strings.Contains(result, "Platform:") {
		t.Error("missing platform info")
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
