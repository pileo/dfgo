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

func TestStartNoIncoming(t *testing.T) {
	// Valid: start has no incoming edges
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
	for _, d := range diags {
		if d.Rule == "start_no_incoming" {
			t.Fatal("unexpected start_no_incoming error on valid graph")
		}
	}

	// Invalid: start has incoming edge
	g2, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		A [shape=box]
		exit [shape=Msquare]
		start -> A -> exit
		A -> start
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g2)
	found := false
	for _, d := range diags {
		if d.Rule == "start_no_incoming" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected start_no_incoming error")
	}
}

func TestExitNoOutgoing(t *testing.T) {
	// Valid: exit has no outgoing edges
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
	for _, d := range diags {
		if d.Rule == "exit_no_outgoing" {
			t.Fatal("unexpected exit_no_outgoing error on valid graph")
		}
	}

	// Invalid: exit has outgoing edge
	g2, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		A [shape=box]
		start -> exit -> A
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g2)
	found := false
	for _, d := range diags {
		if d.Rule == "exit_no_outgoing" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected exit_no_outgoing error")
	}
}

func TestStylesheetSyntax(t *testing.T) {
	// Valid stylesheet
	g, err := dot.Parse(`digraph test {
		graph [model_stylesheet="* { llm_model: gpt-4; }"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	for _, d := range diags {
		if d.Rule == "stylesheet_syntax" {
			t.Fatalf("unexpected stylesheet_syntax error: %s", d.Message)
		}
	}

	// Invalid stylesheet (unclosed brace)
	g2, err := dot.Parse(`digraph test {
		graph [model_stylesheet="* { llm_model: gpt-4;"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g2)
	found := false
	for _, d := range diags {
		if d.Rule == "stylesheet_syntax" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected stylesheet_syntax error")
	}

	// No stylesheet - no error
	g3, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g3)
	for _, d := range diags {
		if d.Rule == "stylesheet_syntax" {
			t.Fatal("unexpected stylesheet_syntax error when no stylesheet present")
		}
	}
}

func TestTypeKnown(t *testing.T) {
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, type="codergen"]
		bad [shape=box, type="nonexistent_type"]
		exit [shape=Msquare]
		start -> work -> bad -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}

	// Without known types - no type_known rule
	r := NewRunner()
	diags := r.Run(g)
	for _, d := range diags {
		if d.Rule == "type_known" {
			t.Fatal("type_known should not fire without known types")
		}
	}

	// With known types - should warn about unknown type
	r2 := NewRunner(WithKnownTypes([]string{"codergen", "tool", "parallel"}))
	diags = r2.Run(g)
	found := false
	for _, d := range diags {
		if d.Rule == "type_known" && d.NodeID == "bad" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected type_known warning for node with unknown type")
	}

	// Known type should not warn
	for _, d := range diags {
		if d.Rule == "type_known" && d.NodeID == "work" {
			t.Fatal("type_known should not warn about known type 'codergen'")
		}
	}
}

func TestFidelityValid(t *testing.T) {
	// Valid fidelity on node
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, fidelity="compact"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	for _, d := range diags {
		if d.Rule == "fidelity_valid" {
			t.Fatal("unexpected fidelity_valid warning for valid mode")
		}
	}

	// Invalid fidelity on node
	g2, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, fidelity="bogus"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g2)
	found := false
	for _, d := range diags {
		if d.Rule == "fidelity_valid" && d.NodeID == "work" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected fidelity_valid warning for invalid mode")
	}

	// Invalid fidelity on graph
	g3, err := dot.Parse(`digraph test {
		graph [fidelity="invalid_mode"]
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g3)
	found = false
	for _, d := range diags {
		if d.Rule == "fidelity_valid" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected fidelity_valid warning for invalid graph-level fidelity")
	}

	// Invalid fidelity on edge
	g4, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		exit [shape=Msquare]
		start -> exit [fidelity="nope"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g4)
	found = false
	for _, d := range diags {
		if d.Rule == "fidelity_valid" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected fidelity_valid warning for invalid edge fidelity")
	}
}

func TestRetryTargetExists(t *testing.T) {
	// Valid: retry_target points to existing node
	g, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, retry_target="start"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner()
	diags := r.Run(g)
	for _, d := range diags {
		if d.Rule == "retry_target_exists" {
			t.Fatal("unexpected retry_target_exists warning for valid target")
		}
	}

	// Invalid: retry_target points to non-existent node
	g2, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, retry_target="ghost"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g2)
	found := false
	for _, d := range diags {
		if d.Rule == "retry_target_exists" && d.NodeID == "work" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected retry_target_exists warning for non-existent target")
	}

	// Invalid: fallback_retry_target points to non-existent node
	g3, err := dot.Parse(`digraph test {
		start [shape=Mdiamond]
		work [shape=box, fallback_retry_target="phantom"]
		exit [shape=Msquare]
		start -> work -> exit
	}`)
	if err != nil {
		t.Fatal(err)
	}
	diags = r.Run(g3)
	found = false
	for _, d := range diags {
		if d.Rule == "retry_target_exists" && d.NodeID == "work" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected retry_target_exists warning for non-existent fallback target")
	}
}
