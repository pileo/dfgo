package fidelity

import (
	"testing"

	"dfgo/internal/attractor/model"
)

func TestModeValid(t *testing.T) {
	valid := []Mode{ModeFull, ModeCompact, ModeSummaryHi, ModeSummaryMed, ModeSummaryLo, ModeTruncate}
	for _, m := range valid {
		if !m.Valid() {
			t.Errorf("expected %q to be valid", m)
		}
	}
	if Mode("bogus").Valid() {
		t.Error("expected bogus to be invalid")
	}
}

func TestResolveEdge(t *testing.T) {
	e := &model.Edge{Attrs: map[string]string{"fidelity": "full"}}
	n := &model.Node{Attrs: map[string]string{"fidelity": "compact"}}
	g := model.NewGraph("test")
	g.Attrs["fidelity"] = "truncate"

	m := Resolve(e, n, g)
	if m != ModeFull {
		t.Fatalf("expected full from edge, got %s", m)
	}
}

func TestResolveNode(t *testing.T) {
	n := &model.Node{Attrs: map[string]string{"fidelity": "summary:high"}}
	g := model.NewGraph("test")

	m := Resolve(nil, n, g)
	if m != ModeSummaryHi {
		t.Fatalf("expected summary:high from node, got %s", m)
	}
}

func TestResolveGraph(t *testing.T) {
	g := model.NewGraph("test")
	g.Attrs["fidelity"] = "truncate"

	m := Resolve(nil, nil, g)
	if m != ModeTruncate {
		t.Fatalf("expected truncate from graph, got %s", m)
	}
}

func TestResolveDefault(t *testing.T) {
	g := model.NewGraph("test")
	m := Resolve(nil, nil, g)
	if m != ModeCompact {
		t.Fatalf("expected compact default, got %s", m)
	}
}
