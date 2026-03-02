package transform

import "testing"

func TestVariableExpandSimple(t *testing.T) {
	v := &VariableExpand{}
	result := v.Apply("Hello $name, your goal is $goal", map[string]string{
		"name": "World",
		"goal": "build something",
	})
	if result != "Hello World, your goal is build something" {
		t.Fatalf("unexpected: %q", result)
	}
}

func TestVariableExpandBraces(t *testing.T) {
	v := &VariableExpand{}
	result := v.Apply("${greeting} ${name}!", map[string]string{
		"greeting": "Hello",
		"name":     "World",
	})
	if result != "Hello World!" {
		t.Fatalf("unexpected: %q", result)
	}
}

func TestVariableExpandMissing(t *testing.T) {
	v := &VariableExpand{}
	result := v.Apply("Hello $missing", map[string]string{})
	if result != "Hello $missing" {
		t.Fatalf("expected unresolved var to remain, got %q", result)
	}
}

func TestVariableExpandDotted(t *testing.T) {
	v := &VariableExpand{}
	result := v.Apply("Result: $step_a.response", map[string]string{
		"step_a.response": "42",
	})
	if result != "Result: 42" {
		t.Fatalf("unexpected: %q", result)
	}
}

func TestRunnerApply(t *testing.T) {
	r := NewRunner()
	result := r.Apply("Goal: $goal", map[string]string{"goal": "test"})
	if result != "Goal: test" {
		t.Fatalf("unexpected: %q", result)
	}
}

func TestRunnerEmpty(t *testing.T) {
	r := &Runner{}
	result := r.Apply("unchanged", nil)
	if result != "unchanged" {
		t.Fatalf("unexpected: %q", result)
	}
}
