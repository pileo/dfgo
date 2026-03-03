package fidelity

import (
	"fmt"
	"strings"

	"dfgo/internal/attractor/runtime"
)

// GeneratePreamble builds a context preamble string based on the fidelity mode.
// It summarizes previous execution state for context carryover:
//   - truncate: only goal + run ID
//   - compact: bullet-point summary of completed stages + outcomes
//   - summary:low / summary:medium / summary:high: proportional detail
//   - full: empty preamble (session reuse / no truncation needed)
func GeneratePreamble(mode Mode, runID string, goal string, visitLog []runtime.VisitEntry) string {
	switch mode {
	case ModeTruncate:
		return preambleTruncate(runID, goal)
	case ModeCompact:
		return preambleCompact(runID, goal, visitLog)
	case ModeSummaryLo:
		return preambleSummary(runID, goal, visitLog, 600)
	case ModeSummaryMed:
		return preambleSummary(runID, goal, visitLog, 1500)
	case ModeSummaryHi:
		return preambleSummary(runID, goal, visitLog, 3000)
	case ModeFull:
		return ""
	default:
		return preambleCompact(runID, goal, visitLog)
	}
}

func preambleTruncate(runID, goal string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Run: %s\n", runID))
	if goal != "" {
		b.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	}
	return b.String()
}

func preambleCompact(runID, goal string, visitLog []runtime.VisitEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Run: %s\n", runID))
	if goal != "" {
		b.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	}
	if len(visitLog) > 0 {
		b.WriteString("Completed stages:\n")
		for _, v := range visitLog {
			b.WriteString(fmt.Sprintf("- %s: %s", v.NodeID, v.Status))
			if v.Attempt > 1 {
				b.WriteString(fmt.Sprintf(" (attempt %d)", v.Attempt))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func preambleSummary(runID, goal string, visitLog []runtime.VisitEntry, tokenBudget int) string {
	// Approximate 4 chars per token for budget calculation.
	charBudget := tokenBudget * 4

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Run: %s\n", runID))
	if goal != "" {
		b.WriteString(fmt.Sprintf("Goal: %s\n", goal))
	}

	if len(visitLog) == 0 {
		return b.String()
	}

	b.WriteString("\nExecution history:\n")

	// Build entries and truncate to fit budget.
	var entries []string
	for _, v := range visitLog {
		entry := fmt.Sprintf("- %s: %s", v.NodeID, v.Status)
		if v.Attempt > 1 {
			entry += fmt.Sprintf(" (attempt %d)", v.Attempt)
		}
		entries = append(entries, entry)
	}

	// Add entries until budget is exhausted.
	for _, entry := range entries {
		if b.Len()+len(entry)+1 > charBudget {
			remaining := len(entries) - len(entries) // simplified
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("- ... and %d more stages\n", remaining))
			}
			break
		}
		b.WriteString(entry + "\n")
	}

	result := b.String()
	if len(result) > charBudget {
		result = result[:charBudget]
	}
	return result
}
