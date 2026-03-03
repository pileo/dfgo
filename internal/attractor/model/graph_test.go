package model

import "testing"

func buildTestGraph() *Graph {
	g := NewGraph("test")
	g.AddNode(&Node{ID: "start", Attrs: map[string]string{"shape": "Mdiamond"}, Order: 0})
	g.AddNode(&Node{ID: "A", Attrs: map[string]string{"shape": "box", "max_retries": "3", "enabled": "true"}, Order: 1})
	g.AddNode(&Node{ID: "B", Attrs: map[string]string{"shape": "box"}, Order: 2})
	g.AddNode(&Node{ID: "exit", Attrs: map[string]string{"shape": "Msquare"}, Order: 3})

	g.AddEdge(&Edge{From: "start", To: "A", Attrs: map[string]string{"label": "begin"}, Order: 0})
	g.AddEdge(&Edge{From: "A", To: "B", Attrs: map[string]string{"weight": "10"}, Order: 1})
	g.AddEdge(&Edge{From: "A", To: "exit", Attrs: map[string]string{"label": "skip"}, Order: 2})
	g.AddEdge(&Edge{From: "B", To: "exit", Order: 3})
	return g
}

func TestNodeByID(t *testing.T) {
	g := buildTestGraph()
	n := g.NodeByID("A")
	if n == nil || n.ID != "A" {
		t.Fatal("expected to find node A")
	}
	if g.NodeByID("nonexistent") != nil {
		t.Fatal("expected nil for missing node")
	}
}

func TestOutEdges(t *testing.T) {
	g := buildTestGraph()
	out := g.OutEdges("A")
	if len(out) != 2 {
		t.Fatalf("expected 2 out-edges from A, got %d", len(out))
	}
	if out[0].To != "B" || out[1].To != "exit" {
		t.Fatal("out-edges not sorted by order")
	}
}

func TestInEdges(t *testing.T) {
	g := buildTestGraph()
	in := g.InEdges("exit")
	if len(in) != 2 {
		t.Fatalf("expected 2 in-edges to exit, got %d", len(in))
	}
}

func TestStartNode(t *testing.T) {
	g := buildTestGraph()
	s := g.StartNode()
	if s == nil || s.ID != "start" {
		t.Fatal("expected start node with Mdiamond shape")
	}
}

func TestExitNodes(t *testing.T) {
	g := buildTestGraph()
	exits := g.ExitNodes()
	if len(exits) != 1 || exits[0].ID != "exit" {
		t.Fatal("expected one exit node with Msquare shape")
	}
}

func TestSuccessors(t *testing.T) {
	g := buildTestGraph()
	succ := g.Successors("A")
	if len(succ) != 2 {
		t.Fatalf("expected 2 successors, got %d", len(succ))
	}
}

func TestPredecessors(t *testing.T) {
	g := buildTestGraph()
	pred := g.Predecessors("exit")
	if len(pred) != 2 {
		t.Fatalf("expected 2 predecessors, got %d", len(pred))
	}
}

func TestNodeIntAttr(t *testing.T) {
	n := &Node{Attrs: map[string]string{"max_retries": "3", "bad": "xyz"}}
	if n.IntAttr("max_retries", 0) != 3 {
		t.Fatal("expected 3")
	}
	if n.IntAttr("missing", 5) != 5 {
		t.Fatal("expected default 5")
	}
	if n.IntAttr("bad", 7) != 7 {
		t.Fatal("expected default 7 for unparseable")
	}
}

func TestNodeBoolAttr(t *testing.T) {
	n := &Node{Attrs: map[string]string{"enabled": "true", "bad": "xyz"}}
	if !n.BoolAttr("enabled", false) {
		t.Fatal("expected true")
	}
	if n.BoolAttr("missing", false) {
		t.Fatal("expected default false")
	}
	if n.BoolAttr("bad", false) {
		t.Fatal("expected default false for unparseable")
	}
}

func TestNodeStringAttr(t *testing.T) {
	n := &Node{Attrs: map[string]string{"name": "hello"}}
	if n.StringAttr("name", "") != "hello" {
		t.Fatal("expected hello")
	}
	if n.StringAttr("missing", "default") != "default" {
		t.Fatal("expected default")
	}
}

func TestAddNodeMerge(t *testing.T) {
	g := NewGraph("test")
	g.AddNode(&Node{ID: "A", Attrs: map[string]string{"shape": "box"}, Order: 0})
	g.AddNode(&Node{ID: "A", Attrs: map[string]string{"label": "hello"}, Order: 1})
	if len(g.Nodes) != 1 {
		t.Fatal("expected merge, not duplicate")
	}
	n := g.NodeByID("A")
	if n.Attrs["shape"] != "box" || n.Attrs["label"] != "hello" {
		t.Fatal("expected merged attrs")
	}
}

func TestEdgeAttrHelpers(t *testing.T) {
	e := &Edge{Attrs: map[string]string{"weight": "10", "label": "ok"}}
	if e.IntAttr("weight", 0) != 10 {
		t.Fatal("expected 10")
	}
	if e.StringAttr("label", "") != "ok" {
		t.Fatal("expected ok")
	}
	if e.FloatAttr("weight", 0) != 10.0 {
		t.Fatal("expected 10.0")
	}
}

func TestNodeString(t *testing.T) {
	n := &Node{ID: "foo"}
	if n.String() != "Node(foo)" {
		t.Fatalf("unexpected: %s", n.String())
	}
}

func TestEdgeString(t *testing.T) {
	e := &Edge{From: "A", To: "B"}
	if e.String() != "Edge(A -> B)" {
		t.Fatalf("unexpected: %s", e.String())
	}
}

func TestGraphIntAttr(t *testing.T) {
	g := NewGraph("test")
	g.Attrs["default_max_retry"] = "50"
	g.Attrs["bad"] = "xyz"

	if g.IntAttr("default_max_retry", 0) != 50 {
		t.Fatal("expected 50")
	}
	if g.IntAttr("missing", 10) != 10 {
		t.Fatal("expected default 10")
	}
	if g.IntAttr("bad", 7) != 7 {
		t.Fatal("expected default 7 for unparseable")
	}
}

func TestGraphBoolAttr(t *testing.T) {
	g := NewGraph("test")
	g.Attrs["verbose"] = "true"
	g.Attrs["bad"] = "xyz"

	if !g.BoolAttr("verbose", false) {
		t.Fatal("expected true")
	}
	if g.BoolAttr("missing", false) {
		t.Fatal("expected default false")
	}
	if g.BoolAttr("bad", false) {
		t.Fatal("expected default false for unparseable")
	}
}

func TestGraphStringAttr(t *testing.T) {
	g := NewGraph("test")
	g.Attrs["goal"] = "build it"

	if g.StringAttr("goal", "") != "build it" {
		t.Fatal("expected 'build it'")
	}
	if g.StringAttr("missing", "default") != "default" {
		t.Fatal("expected default")
	}
}

func TestGraphFloatAttr(t *testing.T) {
	g := NewGraph("test")
	g.Attrs["threshold"] = "0.75"
	g.Attrs["bad"] = "xyz"

	if g.FloatAttr("threshold", 0) != 0.75 {
		t.Fatal("expected 0.75")
	}
	if g.FloatAttr("missing", 1.5) != 1.5 {
		t.Fatal("expected default 1.5")
	}
	if g.FloatAttr("bad", 2.0) != 2.0 {
		t.Fatal("expected default 2.0 for unparseable")
	}
}
