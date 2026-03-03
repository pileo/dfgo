package interviewer

import (
	"fmt"
	"strings"
	"testing"
)

func TestAutoApprove(t *testing.T) {
	a := &AutoApprove{}
	ans, err := a.Ask(Question{Type: YesNo, Prompt: "Continue?"})
	if err != nil || ans.Text != "yes" {
		t.Fatal("expected yes")
	}

	ans, err = a.Ask(Question{Type: MultipleChoice, Options: []string{"[A] Alpha", "[B] Beta"}})
	if err != nil || ans.Selected != 0 {
		t.Fatal("expected first option selected")
	}
}

func TestQueue(t *testing.T) {
	q := NewQueue("yes", "no", "maybe")
	ans, err := q.Ask(Question{Type: YesNo})
	if err != nil || ans.Text != "yes" {
		t.Fatal("expected yes")
	}
	ans, _ = q.Ask(Question{Type: YesNo})
	if ans.Text != "no" {
		t.Fatal("expected no")
	}
	if q.Remaining() != 1 {
		t.Fatal("expected 1 remaining")
	}
	q.Ask(Question{})
	_, err = q.Ask(Question{})
	if err == nil {
		t.Fatal("expected exhausted error")
	}
}

func TestRecording(t *testing.T) {
	inner := NewQueue("yes")
	rec := NewRecording(inner)
	ans, err := rec.Ask(Question{Type: YesNo, Prompt: "OK?"})
	if err != nil || ans.Text != "yes" {
		t.Fatal("expected yes")
	}
	if len(rec.Interactions) != 1 {
		t.Fatal("expected 1 interaction")
	}
	if rec.Interactions[0].Question.Prompt != "OK?" {
		t.Fatal("expected recorded question")
	}
}

func TestParseAccelerator(t *testing.T) {
	tests := []struct {
		input string
		key   string
		clean string
	}{
		{"[Y] Yes", "Y", "Yes"},
		{"[N] No", "N", "No"},
		{"(A) Alpha", "A", "Alpha"},
		{"B) Beta", "B", "Beta"},
		{"No accelerator", "", "No accelerator"},
	}
	for _, tt := range tests {
		key, clean := ParseAccelerator(tt.input)
		if key != tt.key || clean != tt.clean {
			t.Errorf("ParseAccelerator(%q) = (%q, %q), want (%q, %q)", tt.input, key, clean, tt.key, tt.clean)
		}
	}
}

func TestMatchAccelerator(t *testing.T) {
	opts := []string{"[Y] Yes", "[N] No", "Maybe"}
	if MatchAccelerator("y", opts) != 0 {
		t.Fatal("expected Y to match index 0")
	}
	if MatchAccelerator("N", opts) != 1 {
		t.Fatal("expected N to match index 1")
	}
	if MatchAccelerator("M", opts) != -1 {
		t.Fatal("expected M to not match")
	}
}

func TestConsoleYesNo(t *testing.T) {
	var out strings.Builder
	c := &Console{In: strings.NewReader("y\n"), Out: &out}
	ans, err := c.Ask(Question{Type: YesNo, Prompt: "Continue?"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Text != "y" {
		t.Fatalf("expected y, got %q", ans.Text)
	}
}

func TestConsoleMultipleChoice(t *testing.T) {
	var out strings.Builder
	c := &Console{In: strings.NewReader("2\n"), Out: &out}
	ans, err := c.Ask(Question{Type: MultipleChoice, Prompt: "Pick:", Options: []string{"A", "B"}})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Selected != 1 || ans.Text != "B" {
		t.Fatalf("expected B at index 1, got %q at %d", ans.Text, ans.Selected)
	}
}

func TestCallbackBasic(t *testing.T) {
	cb := NewCallback(func(q Question) (Answer, error) {
		return Answer{Text: "yes", Selected: -1}, nil
	})
	ans, err := cb.Ask(Question{Type: YesNo, Prompt: "Continue?"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Text != "yes" || ans.Selected != -1 {
		t.Fatalf("expected yes/-1, got %q/%d", ans.Text, ans.Selected)
	}
}

func TestCallbackError(t *testing.T) {
	cb := NewCallback(func(q Question) (Answer, error) {
		return Answer{}, fmt.Errorf("user cancelled")
	})
	_, err := cb.Ask(Question{Type: Freeform, Prompt: "Name?"})
	if err == nil || err.Error() != "user cancelled" {
		t.Fatalf("expected user cancelled error, got %v", err)
	}
}

func TestCallbackMultipleChoice(t *testing.T) {
	cb := NewCallback(func(q Question) (Answer, error) {
		return Answer{Text: q.Options[1], Selected: 1}, nil
	})
	ans, err := cb.Ask(Question{
		Type:    MultipleChoice,
		Prompt:  "Pick:",
		Options: []string{"Alpha", "Beta"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Text != "Beta" || ans.Selected != 1 {
		t.Fatalf("expected Beta/1, got %q/%d", ans.Text, ans.Selected)
	}
}

func TestCallbackWithRecording(t *testing.T) {
	cb := NewCallback(func(q Question) (Answer, error) {
		return Answer{Text: "confirmed", Selected: -1}, nil
	})
	rec := NewRecording(cb)
	ans, err := rec.Ask(Question{Type: Confirmation, Prompt: "Sure?"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Text != "confirmed" {
		t.Fatalf("expected confirmed, got %q", ans.Text)
	}
	if len(rec.Interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(rec.Interactions))
	}
	if rec.Interactions[0].Question.Prompt != "Sure?" {
		t.Fatal("expected recorded question")
	}
}

func TestConsoleAccelerator(t *testing.T) {
	var out strings.Builder
	c := &Console{In: strings.NewReader("N\n"), Out: &out}
	ans, err := c.Ask(Question{Type: MultipleChoice, Prompt: "Pick:", Options: []string{"[Y] Yes", "[N] No"}})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Selected != 1 {
		t.Fatalf("expected index 1 via accelerator, got %d", ans.Selected)
	}
}
