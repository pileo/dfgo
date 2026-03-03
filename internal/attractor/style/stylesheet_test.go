package style

import (
	"testing"

	"dfgo/internal/attractor/model"
)

func TestParseSelector(t *testing.T) {
	tests := []struct {
		input string
		typ   string
		val   string
		spec  int
	}{
		{"*", "*", "", SpecUniversal},
		{"#myNode", "#", "myNode", SpecID},
		{".box", ".", "box", SpecClass},
		{"box", ".", "box", SpecClass},
	}
	for _, tt := range tests {
		sel := ParseSelector(tt.input)
		if sel.Type != tt.typ || sel.Value != tt.val || sel.Spec != tt.spec {
			t.Errorf("ParseSelector(%q) = %+v, want type=%s val=%s spec=%d", tt.input, sel, tt.typ, tt.val, tt.spec)
		}
	}
}

func TestSelectorMatches(t *testing.T) {
	n := &model.Node{ID: "myNode", Attrs: map[string]string{"shape": "box"}}

	if !ParseSelector("*").Matches(n) {
		t.Fatal("universal should match")
	}
	if !ParseSelector("#myNode").Matches(n) {
		t.Fatal("ID should match")
	}
	if ParseSelector("#other").Matches(n) {
		t.Fatal("wrong ID should not match")
	}
	if !ParseSelector(".box").Matches(n) {
		t.Fatal("shape class should match")
	}
	if ParseSelector(".diamond").Matches(n) {
		t.Fatal("wrong shape should not match")
	}
}

func TestParseStylesheet(t *testing.T) {
	src := `
* {
    llm_model: gpt-4;
    reasoning_effort: medium;
}
.box {
    llm_model: claude-3;
}
#special {
    reasoning_effort: high;
}
`
	ss, err := ParseStylesheet(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ss.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(ss.Rules))
	}
	if ss.Rules[0].Selector.Type != "*" {
		t.Fatal("expected universal selector first")
	}
	if ss.Rules[1].Properties["llm_model"] != "claude-3" {
		t.Fatal("expected llm_model=claude-3 for .box")
	}
}

func TestParseStylesheetErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unclosed brace", `* { llm_model: gpt-4;`},
		{"empty selector", `{ llm_model: gpt-4; }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseStylesheet(tt.src)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestResolve(t *testing.T) {
	src := `
* {
    llm_model: default;
    reasoning_effort: low;
}
.box {
    llm_model: claude-3;
}
#special {
    reasoning_effort: high;
}
`
	ss, err := ParseStylesheet(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Generic box node
	n := &model.Node{ID: "generic", Attrs: map[string]string{"shape": "box"}}
	props := ss.Resolve(n)
	if props["llm_model"] != "claude-3" {
		t.Fatalf("expected claude-3 for box, got %s", props["llm_model"])
	}
	if props["reasoning_effort"] != "low" {
		t.Fatalf("expected low effort for generic box, got %s", props["reasoning_effort"])
	}

	// Specific node
	n2 := &model.Node{ID: "special", Attrs: map[string]string{"shape": "box"}}
	props2 := ss.Resolve(n2)
	if props2["reasoning_effort"] != "high" {
		t.Fatalf("expected high effort for #special, got %s", props2["reasoning_effort"])
	}
	if props2["llm_model"] != "claude-3" {
		t.Fatalf("expected claude-3 for #special (box shape), got %s", props2["llm_model"])
	}
}

func TestResolveEmpty(t *testing.T) {
	ss := Stylesheet{}
	n := &model.Node{ID: "any", Attrs: map[string]string{}}
	props := ss.Resolve(n)
	if len(props) != 0 {
		t.Fatal("expected empty props for empty stylesheet")
	}
}

func TestSelectorMatchesClass(t *testing.T) {
	// Node with class attribute (comma-separated).
	n := &model.Node{ID: "n1", Attrs: map[string]string{
		"shape": "box",
		"class": "agent, worker",
	}}

	if !ParseSelector(".agent").Matches(n) {
		t.Fatal("class 'agent' should match")
	}
	if !ParseSelector(".worker").Matches(n) {
		t.Fatal("class 'worker' should match")
	}
	if !ParseSelector(".box").Matches(n) {
		t.Fatal("shape 'box' should still match")
	}
	if ParseSelector(".manager").Matches(n) {
		t.Fatal("class 'manager' should not match")
	}

	// Node with no class.
	n2 := &model.Node{ID: "n2", Attrs: map[string]string{"shape": "diamond"}}
	if ParseSelector(".agent").Matches(n2) {
		t.Fatal("should not match without class attr")
	}
}

func TestApply(t *testing.T) {
	src := `
* {
    llm_model: gpt-4;
    temperature: 0.7;
}
.box {
    llm_model: claude-3;
}
`
	ss, err := ParseStylesheet(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	g := model.NewGraph("test")
	// Node with explicit llm_model should not be overridden.
	n1 := &model.Node{ID: "n1", Attrs: map[string]string{
		"shape":     "box",
		"llm_model": "gemini",
	}}
	// Node without explicit attrs should get stylesheet values.
	n2 := &model.Node{ID: "n2", Attrs: map[string]string{
		"shape": "diamond",
	}}
	g.AddNode(n1)
	g.AddNode(n2)

	ss.Apply(g)

	// n1: explicit llm_model preserved, temperature added.
	if n1.Attrs["llm_model"] != "gemini" {
		t.Fatalf("explicit attr should not be overridden, got %s", n1.Attrs["llm_model"])
	}
	if n1.Attrs["temperature"] != "0.7" {
		t.Fatalf("expected temperature=0.7, got %s", n1.Attrs["temperature"])
	}

	// n2: gets universal defaults.
	if n2.Attrs["llm_model"] != "gpt-4" {
		t.Fatalf("expected gpt-4 from universal, got %s", n2.Attrs["llm_model"])
	}
	if n2.Attrs["temperature"] != "0.7" {
		t.Fatalf("expected temperature=0.7, got %s", n2.Attrs["temperature"])
	}
}

func TestApplyNoOverrideExplicit(t *testing.T) {
	src := `#myNode { fidelity: full; }`
	ss, err := ParseStylesheet(src)
	if err != nil {
		t.Fatal(err)
	}

	g := model.NewGraph("test")
	n := &model.Node{ID: "myNode", Attrs: map[string]string{"fidelity": "compact"}}
	g.AddNode(n)

	ss.Apply(g)

	if n.Attrs["fidelity"] != "compact" {
		t.Fatalf("explicit fidelity should not be overridden, got %s", n.Attrs["fidelity"])
	}
}
