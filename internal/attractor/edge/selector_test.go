package edge

import (
	"testing"

	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

func buildEdgeTestGraph() *model.Graph {
	g := model.NewGraph("test")
	g.AddNode(&model.Node{ID: "A", Attrs: map[string]string{}, Order: 0})
	g.AddNode(&model.Node{ID: "B", Attrs: map[string]string{}, Order: 1})
	g.AddNode(&model.Node{ID: "C", Attrs: map[string]string{}, Order: 2})
	g.AddNode(&model.Node{ID: "D", Attrs: map[string]string{}, Order: 3})
	return g
}

func TestSelectConditionMatch(t *testing.T) {
	g := buildEdgeTestGraph()
	g.AddEdge(&model.Edge{From: "A", To: "B", Attrs: map[string]string{"condition": "outcome=SUCCESS"}, Order: 0})
	g.AddEdge(&model.Edge{From: "A", To: "C", Attrs: map[string]string{"condition": "outcome=FAIL"}, Order: 1})

	ctx := runtime.NewContext()
	e := Select(g, "A", runtime.Outcome{Status: runtime.StatusSuccess}, ctx)
	if e == nil || e.To != "B" {
		t.Fatal("expected edge to B on SUCCESS")
	}

	e = Select(g, "A", runtime.Outcome{Status: runtime.StatusFail}, ctx)
	if e == nil || e.To != "C" {
		t.Fatal("expected edge to C on FAIL")
	}
}

func TestSelectPreferredLabel(t *testing.T) {
	g := buildEdgeTestGraph()
	g.AddEdge(&model.Edge{From: "A", To: "B", Attrs: map[string]string{"label": "yes"}, Order: 0})
	g.AddEdge(&model.Edge{From: "A", To: "C", Attrs: map[string]string{"label": "no"}, Order: 1})

	ctx := runtime.NewContext()
	e := Select(g, "A", runtime.Outcome{Status: runtime.StatusSuccess, PreferredLabel: "no"}, ctx)
	if e == nil || e.To != "C" {
		t.Fatal("expected edge to C via preferred label 'no'")
	}
}

func TestSelectSuggestedNextID(t *testing.T) {
	g := buildEdgeTestGraph()
	g.AddEdge(&model.Edge{From: "A", To: "B", Attrs: map[string]string{}, Order: 0})
	g.AddEdge(&model.Edge{From: "A", To: "C", Attrs: map[string]string{}, Order: 1})

	ctx := runtime.NewContext()
	e := Select(g, "A", runtime.Outcome{Status: runtime.StatusSuccess, SuggestedNextID: "C"}, ctx)
	if e == nil || e.To != "C" {
		t.Fatal("expected edge to C via suggested next ID")
	}
}

func TestSelectHighestWeight(t *testing.T) {
	g := buildEdgeTestGraph()
	g.AddEdge(&model.Edge{From: "A", To: "B", Attrs: map[string]string{"weight": "1"}, Order: 0})
	g.AddEdge(&model.Edge{From: "A", To: "C", Attrs: map[string]string{"weight": "10"}, Order: 1})

	ctx := runtime.NewContext()
	e := Select(g, "A", runtime.Outcome{Status: runtime.StatusSuccess}, ctx)
	if e == nil || e.To != "C" {
		t.Fatalf("expected edge to C (highest weight), got %v", e)
	}
}

func TestSelectDeclarationOrder(t *testing.T) {
	g := buildEdgeTestGraph()
	g.AddEdge(&model.Edge{From: "A", To: "B", Attrs: map[string]string{}, Order: 0})
	g.AddEdge(&model.Edge{From: "A", To: "C", Attrs: map[string]string{}, Order: 1})

	ctx := runtime.NewContext()
	e := Select(g, "A", runtime.Outcome{Status: runtime.StatusSuccess}, ctx)
	if e == nil || e.To != "B" {
		t.Fatal("expected first edge by declaration order")
	}
}

func TestSelectNoEdges(t *testing.T) {
	g := buildEdgeTestGraph()
	ctx := runtime.NewContext()
	e := Select(g, "D", runtime.Outcome{Status: runtime.StatusSuccess}, ctx)
	if e != nil {
		t.Fatal("expected nil for node with no out-edges")
	}
}

func TestSelectConditionTakesPriority(t *testing.T) {
	g := buildEdgeTestGraph()
	// Unconditional edge with higher weight
	g.AddEdge(&model.Edge{From: "A", To: "B", Attrs: map[string]string{"weight": "100"}, Order: 0})
	// Conditional edge
	g.AddEdge(&model.Edge{From: "A", To: "C", Attrs: map[string]string{"condition": "outcome=SUCCESS"}, Order: 1})

	ctx := runtime.NewContext()
	e := Select(g, "A", runtime.Outcome{Status: runtime.StatusSuccess}, ctx)
	if e == nil || e.To != "C" {
		t.Fatal("condition-matching edge should take priority over weight")
	}
}
