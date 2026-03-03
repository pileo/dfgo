package fidelity

import (
	"strings"
	"testing"

	"dfgo/internal/attractor/runtime"
)

func TestGeneratePreambleTruncate(t *testing.T) {
	result := GeneratePreamble(ModeTruncate, "run-123", "build feature X", nil)

	if !strings.Contains(result, "run-123") {
		t.Fatal("truncate preamble should contain run ID")
	}
	if !strings.Contains(result, "build feature X") {
		t.Fatal("truncate preamble should contain goal")
	}
	// Should NOT contain visit log details.
	if strings.Contains(result, "stages") {
		t.Fatal("truncate preamble should not contain stage details")
	}
}

func TestGeneratePreambleCompact(t *testing.T) {
	visits := []runtime.VisitEntry{
		{NodeID: "start", Status: runtime.StatusSuccess, Attempt: 1},
		{NodeID: "build", Status: runtime.StatusSuccess, Attempt: 2},
		{NodeID: "test", Status: runtime.StatusFail, Attempt: 1},
	}

	result := GeneratePreamble(ModeCompact, "run-456", "deploy", visits)

	if !strings.Contains(result, "run-456") {
		t.Fatal("compact preamble should contain run ID")
	}
	if !strings.Contains(result, "deploy") {
		t.Fatal("compact preamble should contain goal")
	}
	if !strings.Contains(result, "build") {
		t.Fatal("compact preamble should list stages")
	}
	if !strings.Contains(result, "attempt 2") {
		t.Fatal("compact preamble should show retry attempts")
	}
}

func TestGeneratePreambleCompactNoVisits(t *testing.T) {
	result := GeneratePreamble(ModeCompact, "run-789", "goal", nil)

	if !strings.Contains(result, "run-789") {
		t.Fatal("should contain run ID")
	}
	if strings.Contains(result, "Completed") {
		t.Fatal("should not have completed section with no visits")
	}
}

func TestGeneratePreambleFull(t *testing.T) {
	result := GeneratePreamble(ModeFull, "run-abc", "anything", []runtime.VisitEntry{
		{NodeID: "a", Status: runtime.StatusSuccess, Attempt: 1},
	})

	if result != "" {
		t.Fatalf("full mode should return empty preamble, got %q", result)
	}
}

func TestGeneratePreambleSummaryModes(t *testing.T) {
	visits := []runtime.VisitEntry{
		{NodeID: "start", Status: runtime.StatusSuccess, Attempt: 1},
		{NodeID: "process", Status: runtime.StatusSuccess, Attempt: 1},
	}

	for _, mode := range []Mode{ModeSummaryLo, ModeSummaryMed, ModeSummaryHi} {
		result := GeneratePreamble(mode, "run-1", "goal", visits)
		if !strings.Contains(result, "run-1") {
			t.Fatalf("mode %s: should contain run ID", mode)
		}
		if !strings.Contains(result, "start") {
			t.Fatalf("mode %s: should contain stage info", mode)
		}
	}
}

func TestGeneratePreambleNoGoal(t *testing.T) {
	result := GeneratePreamble(ModeTruncate, "run-1", "", nil)
	if strings.Contains(result, "Goal:") {
		t.Fatal("should not have Goal line when goal is empty")
	}
}
