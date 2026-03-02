package cond

import "testing"

func TestParseSimpleEquals(t *testing.T) {
	expr, err := Parse("outcome=SUCCESS")
	if err != nil {
		t.Fatal(err)
	}
	if len(expr.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(expr.Clauses))
	}
	c := expr.Clauses[0]
	if c.Key != "outcome" || c.Op != "=" || c.Val != "SUCCESS" {
		t.Fatalf("unexpected clause: %+v", c)
	}
}

func TestParseNotEquals(t *testing.T) {
	expr, err := Parse("outcome!=FAIL")
	if err != nil {
		t.Fatal(err)
	}
	c := expr.Clauses[0]
	if c.Key != "outcome" || c.Op != "!=" || c.Val != "FAIL" {
		t.Fatalf("unexpected clause: %+v", c)
	}
}

func TestParseConjunction(t *testing.T) {
	expr, err := Parse("outcome=SUCCESS && context.approved=true")
	if err != nil {
		t.Fatal(err)
	}
	if len(expr.Clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(expr.Clauses))
	}
	if expr.Clauses[1].Key != "context.approved" {
		t.Fatalf("unexpected second clause key: %q", expr.Clauses[1].Key)
	}
}

func TestParseBareKey(t *testing.T) {
	expr, err := Parse("context.ready")
	if err != nil {
		t.Fatal(err)
	}
	c := expr.Clauses[0]
	if c.Key != "context.ready" || c.Op != "" {
		t.Fatalf("expected truthy check, got %+v", c)
	}
}

func TestParseEmpty(t *testing.T) {
	expr, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if len(expr.Clauses) != 0 {
		t.Fatal("expected empty expression")
	}
}

func TestEvalOutcome(t *testing.T) {
	expr, _ := Parse("outcome=SUCCESS")
	env := Env{Outcome: "SUCCESS"}
	if !expr.Eval(env) {
		t.Fatal("expected true")
	}
	env.Outcome = "FAIL"
	if expr.Eval(env) {
		t.Fatal("expected false")
	}
}

func TestEvalPreferredLabel(t *testing.T) {
	expr, _ := Parse("preferred_label=yes")
	env := Env{PreferredLabel: "yes"}
	if !expr.Eval(env) {
		t.Fatal("expected true")
	}
}

func TestEvalContext(t *testing.T) {
	expr, _ := Parse("context.approved=true")
	env := Env{Context: map[string]string{"approved": "true"}}
	if !expr.Eval(env) {
		t.Fatal("expected true")
	}
}

func TestEvalTruthy(t *testing.T) {
	expr, _ := Parse("context.ready")
	env := Env{Context: map[string]string{"ready": "yes"}}
	if !expr.Eval(env) {
		t.Fatal("expected true for non-empty")
	}
	env.Context["ready"] = ""
	if expr.Eval(env) {
		t.Fatal("expected false for empty")
	}
}

func TestEvalConjunction(t *testing.T) {
	expr, _ := Parse("outcome=SUCCESS && context.flag=on")
	env := Env{Outcome: "SUCCESS", Context: map[string]string{"flag": "on"}}
	if !expr.Eval(env) {
		t.Fatal("expected true")
	}
	env.Context["flag"] = "off"
	if expr.Eval(env) {
		t.Fatal("expected false when one clause fails")
	}
}

func TestEvalNotEquals(t *testing.T) {
	expr, _ := Parse("outcome!=FAIL")
	if !expr.Eval(Env{Outcome: "SUCCESS"}) {
		t.Fatal("expected true")
	}
	if expr.Eval(Env{Outcome: "FAIL"}) {
		t.Fatal("expected false")
	}
}

func TestEvalEmptyAlwaysTrue(t *testing.T) {
	expr, _ := Parse("")
	if !expr.Eval(Env{}) {
		t.Fatal("empty expr should always be true")
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("outcome=SUCCESS"); err != nil {
		t.Fatal(err)
	}
	if err := Validate(""); err != nil {
		t.Fatal(err)
	}
}
