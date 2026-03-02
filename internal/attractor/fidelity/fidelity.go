// Package fidelity defines fidelity modes that control LLM reasoning effort.
package fidelity

import "dfgo/internal/attractor/model"

// Mode represents a fidelity/reasoning effort level.
type Mode string

const (
	ModeFull       Mode = "full"
	ModeCompact    Mode = "compact"
	ModeSummaryHi  Mode = "summary:high"
	ModeSummaryMed Mode = "summary:medium"
	ModeSummaryLo  Mode = "summary:low"
	ModeTruncate   Mode = "truncate"
)

// Valid returns true if the mode is a recognized fidelity mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeFull, ModeCompact, ModeSummaryHi, ModeSummaryMed, ModeSummaryLo, ModeTruncate:
		return true
	}
	return false
}

// Resolve determines the effective fidelity mode using a resolution chain:
// edge attr → node attr → graph attr → default (compact).
func Resolve(edge *model.Edge, node *model.Node, g *model.Graph) Mode {
	// 1. Edge-level override
	if edge != nil {
		if m := Mode(edge.StringAttr("fidelity", "")); m.Valid() {
			return m
		}
	}

	// 2. Node-level
	if node != nil {
		if m := Mode(node.StringAttr("fidelity", "")); m.Valid() {
			return m
		}
	}

	// 3. Graph-level
	if g != nil {
		if m := Mode(g.Attrs["fidelity"]); m.Valid() {
			return m
		}
	}

	// 4. Default
	return ModeCompact
}
