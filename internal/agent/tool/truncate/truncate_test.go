package truncate

import (
	"strings"
	"testing"
)

func TestTruncateNoOp(t *testing.T) {
	out, truncated := Truncate("shell", "hello world")
	if truncated {
		t.Error("should not truncate short output")
	}
	if out != "hello world" {
		t.Errorf("output changed: %q", out)
	}
}

func TestTruncateLines(t *testing.T) {
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = "line"
	}
	input := strings.Join(lines, "\n")
	out, truncated := Truncate("shell", input)
	if !truncated {
		t.Error("should truncate")
	}
	if !strings.Contains(out, "lines omitted") {
		t.Error("should contain omitted marker")
	}
	// Output should be shorter than input.
	if len(out) >= len(input) {
		t.Error("output should be shorter")
	}
}

func TestTruncateChars(t *testing.T) {
	input := strings.Repeat("x", 60000)
	out, truncated := Truncate("read_file", input)
	if !truncated {
		t.Error("should truncate")
	}
	if !strings.Contains(out, "characters omitted") {
		t.Error("should contain omitted marker")
	}
	// MaxChars for read_file is 50000, output should be around that size + marker.
	if len(out) > 55000 {
		t.Errorf("output too long: %d", len(out))
	}
}

func TestTruncateBothPhases(t *testing.T) {
	// Create output with many long lines to trigger both line and char limits.
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = strings.Repeat("x", 200)
	}
	input := strings.Join(lines, "\n")
	out, truncated := Truncate("shell", input)
	if !truncated {
		t.Error("should truncate")
	}
	if !strings.Contains(out, "omitted") {
		t.Error("should contain omitted marker")
	}
}

func TestTruncateUnknownTool(t *testing.T) {
	input := strings.Repeat("x", 40000)
	out, truncated := Truncate("unknown_tool", input)
	if !truncated {
		t.Error("should truncate with fallback limits")
	}
	_ = out
}

func TestTruncateWithCustomLimits(t *testing.T) {
	out, truncated := TruncateWithLimits("hello", Limits{MaxChars: 3, MaxLines: 0})
	if !truncated {
		t.Error("should truncate")
	}
	if !strings.Contains(out, "characters omitted") {
		t.Error("should contain marker")
	}
}

func TestTruncatePreservesHeadAndTail(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat(string(rune('a'+i%26)), 10)
	}
	input := strings.Join(lines, "\n")
	out, _ := TruncateWithLimits(input, Limits{MaxLines: 10})

	// Should contain first few and last few lines.
	if !strings.HasPrefix(out, "aaaaaaaaaa") {
		t.Error("should preserve head")
	}
	if !strings.HasSuffix(out, "tttttttttt") {
		t.Error("should preserve tail")
	}
}
