package dot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLexerBasic(t *testing.T) {
	tokens, err := Tokenize(`digraph G { A -> B [label="hello"] }`)
	if err != nil {
		t.Fatal(err)
	}
	expected := []TokenType{
		TokenDigraph, TokenIdent, TokenLBrace,
		TokenIdent, TokenArrow, TokenIdent,
		TokenLBracket, TokenIdent, TokenEquals, TokenString, TokenRBracket,
		TokenRBrace, TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, tt := range expected {
		if tokens[i].Type != tt {
			t.Errorf("token %d: expected %s, got %s", i, tt, tokens[i].Type)
		}
	}
}

func TestLexerComments(t *testing.T) {
	tokens, err := Tokenize(`digraph G {
		// line comment
		/* block comment */
		# hash comment
		A -> B
	}`)
	if err != nil {
		t.Fatal(err)
	}
	types := make([]TokenType, len(tokens))
	for i, tok := range tokens {
		types[i] = tok.Type
	}
	expected := []TokenType{TokenDigraph, TokenIdent, TokenLBrace, TokenIdent, TokenArrow, TokenIdent, TokenRBrace, TokenEOF}
	if len(types) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(types), types)
	}
}

func TestLexerEscapedString(t *testing.T) {
	tokens, err := Tokenize(`"hello \"world\""`)
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Text != `hello "world"` {
		t.Fatalf("expected escaped string, got %q", tokens[0].Text)
	}
}

func TestParseSimple(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "pipelines", "simple.dot"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "simple" {
		t.Fatalf("expected name 'simple', got %q", g.Name)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.StartNode() == nil {
		t.Fatal("expected start node")
	}
	if len(g.ExitNodes()) != 1 {
		t.Fatal("expected exit node")
	}
}

func TestParseLinear(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "pipelines", "linear.dot"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "linear" {
		t.Fatalf("expected name 'linear', got %q", g.Name)
	}
	if len(g.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(g.Nodes))
	}
	// start -> A -> B -> exit = 3 edges
	if len(g.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(g.Edges))
	}
	// Check edge chain
	if g.Edges[0].From != "start" || g.Edges[0].To != "A" {
		t.Fatal("unexpected first edge")
	}
	// Verify node attrs
	a := g.NodeByID("A")
	if a == nil {
		t.Fatal("missing node A")
	}
	if a.Attrs["type"] != "codergen" {
		t.Fatalf("expected type=codergen, got %q", a.Attrs["type"])
	}
}

func TestParseBranching(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "pipelines", "branching.dot"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(g.Nodes))
	}
	// check -> path_a, check -> path_b
	out := g.OutEdges("check")
	if len(out) != 2 {
		t.Fatalf("expected 2 out-edges from check, got %d", len(out))
	}
	if out[0].Attrs["condition"] != "outcome=SUCCESS" {
		t.Fatalf("expected condition on first edge, got %q", out[0].Attrs["condition"])
	}
}

func TestParseParallel(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "pipelines", "parallel.dot"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	fanOut := g.NodeByID("fan_out")
	if fanOut == nil || fanOut.Attrs["type"] != "parallel" {
		t.Fatal("expected fan_out node with type=parallel")
	}
	out := g.OutEdges("fan_out")
	if len(out) != 2 {
		t.Fatalf("expected 2 children of fan_out, got %d", len(out))
	}
}

func TestParseEdgeChaining(t *testing.T) {
	g, err := Parse(`digraph test { A -> B -> C -> D }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 3 {
		t.Fatalf("expected 3 edges from chain, got %d", len(g.Edges))
	}
	if g.Edges[0].From != "A" || g.Edges[0].To != "B" {
		t.Fatal("wrong first edge in chain")
	}
	if g.Edges[2].From != "C" || g.Edges[2].To != "D" {
		t.Fatal("wrong last edge in chain")
	}
}

func TestParseSubgraph(t *testing.T) {
	g, err := Parse(`digraph test {
		subgraph cluster_0 {
			A [shape=box]
			B [shape=box]
			A -> B
		}
		start [shape=Mdiamond]
		start -> A
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if g.NodeByID("A") == nil || g.NodeByID("B") == nil {
		t.Fatal("expected subgraph nodes flattened into main graph")
	}
	if len(g.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(g.Edges))
	}
}

func TestParseDefaultAttrs(t *testing.T) {
	g, err := Parse(`digraph test {
		node [shape=box]
		A
		B
		A -> B
	}`)
	if err != nil {
		t.Fatal(err)
	}
	a := g.NodeByID("A")
	if a == nil || a.Attrs["shape"] != "box" {
		t.Fatal("expected default shape=box on A")
	}
}

func TestParseGraphLevelAttr(t *testing.T) {
	g, err := Parse(`digraph test {
		graph [goal="build something"]
		rankdir = LR
		A -> B
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if g.Attrs["goal"] != "build something" {
		t.Fatalf("expected graph-level goal attr, got %v", g.Attrs)
	}
	if g.Attrs["rankdir"] != "LR" {
		t.Fatalf("expected graph-level rankdir, got %v", g.Attrs)
	}
}

func TestParseError(t *testing.T) {
	_, err := Parse(`not a digraph`)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseQuotedNodeIDs(t *testing.T) {
	g, err := Parse(`digraph test { "node with spaces" -> B }`)
	if err != nil {
		t.Fatal(err)
	}
	n := g.NodeByID("node with spaces")
	if n == nil {
		t.Fatal("expected node with quoted ID")
	}
}
