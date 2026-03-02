package validate

import (
	"testing"

	"dfgo/internal/attractor/dot"
)

func TestValidateSimple(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	if HasErrors(diags) {
		t.Fatalf("expected no errors, got %v", diags)
	}
}

func TestValidateNoStart(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		A [shape=box]
		exit [shape=Msquare]
		A -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	errs := Errors(diags)
	if len(errs) == 0 {
		t.Fatal("expected error for missing start node")
	}
	found := false
	for _, d := range errs {
		if d.Rule == "start_node" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected start_node error")
	}
}

func TestValidateNoTerminal(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		A [shape=box]
		start -> A
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	errs := Errors(diags)
	found := false
	for _, d := range errs {
		if d.Rule == "terminal_node" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected terminal_node error")
	}
}

func TestValidateUnreachable(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		A [shape=box]
		B [shape=box]
		exit [shape=Msquare]
		start -> A -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	found := false
	for _, d := range diags {
		if d.Rule == "reachability" && d.NodeID == "B" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected reachability warning for B")
	}
}

func TestValidateConditionSyntax(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit [condition="outcome=SUCCESS"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	if HasErrors(diags) {
		t.Fatalf("expected no errors for valid condition, got %v", diags)
	}
}

func TestValidateGoalGateRetry(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, goal_gate="true"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	found := false
	for _, d := range diags {
		if d.Rule == "goal_gate_has_retry" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected goal_gate_has_retry warning")
	}
}

func TestValidatePromptOnLLM(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="codergen"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	found := false
	for _, d := range diags {
		if d.Rule == "prompt_on_llm_nodes" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected prompt_on_llm_nodes warning")
	}
}

func TestHasErrors(t *testing.T) {
	diags := []Diagnostic{
		{Severity: SeverityWarning},
		{Severity: SeverityError},
	}
	if !HasErrors(diags) {
		t.Fatal("expected HasErrors to return true")
	}
	if HasErrors([]Diagnostic{{Severity: SeverityWarning}}) {
		t.Fatal("expected HasErrors to return false")
	}
}
