// Package truncate provides output truncation for tool results.
// Uses a two-phase middle-cut strategy: first enforce character limits,
// then enforce line limits, cutting from the middle to preserve
// the beginning and end of output.
package truncate

import (
	"fmt"
	"strings"
)

// Limits defines truncation thresholds for a specific tool.
type Limits struct {
	MaxChars int
	MaxLines int
}

// DefaultLimits maps tool names to their truncation limits.
var DefaultLimits = map[string]Limits{
	"read_file":  {MaxChars: 50000, MaxLines: 0},
	"shell":      {MaxChars: 30000, MaxLines: 256},
	"grep":       {MaxChars: 20000, MaxLines: 200},
	"glob":       {MaxChars: 20000, MaxLines: 500},
	"edit_file":  {MaxChars: 10000, MaxLines: 0},
	"write_file": {MaxChars: 1000, MaxLines: 0},
}

// FallbackLimits are used when a tool has no specific limits.
var FallbackLimits = Limits{MaxChars: 30000, MaxLines: 256}

// Truncate applies two-phase middle-cut truncation to output.
// Returns the (possibly truncated) output and whether truncation occurred.
func Truncate(toolName, output string) (string, bool) {
	limits, ok := DefaultLimits[toolName]
	if !ok {
		limits = FallbackLimits
	}
	return TruncateWithLimits(output, limits)
}

// TruncateWithLimits applies truncation with explicit limits.
func TruncateWithLimits(output string, limits Limits) (string, bool) {
	truncated := false

	// Phase 1: Character truncation (middle-cut).
	if limits.MaxChars > 0 && len(output) > limits.MaxChars {
		keep := limits.MaxChars
		head := keep / 2
		tail := keep - head
		omitted := len(output) - keep
		output = output[:head] +
			fmt.Sprintf("\n... [%d characters omitted] ...\n", omitted) +
			output[len(output)-tail:]
		truncated = true
	}

	// Phase 2: Line truncation (middle-cut).
	if limits.MaxLines > 0 {
		lines := strings.Split(output, "\n")
		if len(lines) > limits.MaxLines {
			keep := limits.MaxLines
			head := keep / 2
			tail := keep - head
			omitted := len(lines) - keep
			result := make([]string, 0, keep+1)
			result = append(result, lines[:head]...)
			result = append(result, fmt.Sprintf("\n... [%d lines omitted] ...\n", omitted))
			result = append(result, lines[len(lines)-tail:]...)
			output = strings.Join(result, "\n")
			truncated = true
		}
	}

	return output, truncated
}
