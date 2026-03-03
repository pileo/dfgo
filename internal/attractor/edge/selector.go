// Package edge implements the 5-step edge selection priority for Attractor pipelines.
package edge

import (
	"sort"

	"dfgo/internal/attractor/cond"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// Select picks the next edge from the outgoing edges of a node using a 5-step priority:
//  1. Condition-matching edges (first match wins among those with conditions)
//  2. Edge whose label matches outcome.PreferredLabel
//  3. Edge whose target matches one of outcome.SuggestedNextIDs (first match wins)
//  4. Highest-weight unconditional edge
//  5. Lexical tiebreak (first by declaration order)
//
// Returns nil if no edge can be selected.
func Select(g *model.Graph, nodeID string, outcome runtime.Outcome, ctx *runtime.Context) *model.Edge {
	edges := g.OutEdges(nodeID)
	if len(edges) == 0 {
		return nil
	}

	env := cond.Env{
		Outcome:        string(outcome.Status),
		PreferredLabel: outcome.PreferredLabel,
		Context:        ctx.Snapshot(),
	}

	// Step 1: condition-matching edges
	for _, e := range edges {
		condStr := e.Attrs["condition"]
		if condStr == "" {
			continue
		}
		expr, err := cond.Parse(condStr)
		if err != nil {
			continue
		}
		if expr.Eval(env) {
			return e
		}
	}

	// Step 2: preferred label
	if outcome.PreferredLabel != "" {
		for _, e := range edges {
			if e.Attrs["label"] == outcome.PreferredLabel {
				return e
			}
		}
	}

	// Step 3: suggested next IDs (priority order)
	for _, suggestedID := range outcome.SuggestedNextIDs {
		for _, e := range edges {
			if e.To == suggestedID {
				return e
			}
		}
	}

	// Step 4: highest-weight unconditional edge
	unconditional := filterUnconditional(edges)
	if len(unconditional) > 0 {
		sort.Slice(unconditional, func(i, j int) bool {
			wi := unconditional[i].FloatAttr("weight", 0)
			wj := unconditional[j].FloatAttr("weight", 0)
			if wi != wj {
				return wi > wj // higher weight first
			}
			return unconditional[i].Order < unconditional[j].Order
		})
		return unconditional[0]
	}

	// Step 5: lexical tiebreak — first by declaration order (already sorted)
	return edges[0]
}

func filterUnconditional(edges []*model.Edge) []*model.Edge {
	var out []*model.Edge
	for _, e := range edges {
		if e.Attrs["condition"] == "" {
			out = append(out, e)
		}
	}
	return out
}
