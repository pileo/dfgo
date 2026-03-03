// Package model defines the immutable graph data structures for Attractor pipelines.
package model

import (
	"fmt"
	"sort"
	"strconv"
)

// Graph represents a parsed DOT digraph.
type Graph struct {
	Name  string
	Nodes []*Node
	Edges []*Edge
	Attrs map[string]string // graph-level attributes

	nodesByID map[string]*Node
}

// Node represents a pipeline stage.
type Node struct {
	ID    string
	Attrs map[string]string
	Order int // declaration order for deterministic traversal
}

// Edge represents a transition between stages.
type Edge struct {
	From  string
	To    string
	Attrs map[string]string
	Order int // declaration order for deterministic traversal
}

// NewGraph creates a Graph with the given name.
func NewGraph(name string) *Graph {
	return &Graph{
		Name:      name,
		Attrs:     make(map[string]string),
		nodesByID: make(map[string]*Node),
	}
}

// AddNode adds a node to the graph. If a node with the same ID already exists,
// its attributes are merged (new attrs overwrite existing).
func (g *Graph) AddNode(n *Node) {
	if existing, ok := g.nodesByID[n.ID]; ok {
		for k, v := range n.Attrs {
			existing.Attrs[k] = v
		}
		return
	}
	if n.Attrs == nil {
		n.Attrs = make(map[string]string)
	}
	g.nodesByID[n.ID] = n
	g.Nodes = append(g.Nodes, n)
}

// AddEdge adds an edge to the graph.
func (g *Graph) AddEdge(e *Edge) {
	if e.Attrs == nil {
		e.Attrs = make(map[string]string)
	}
	g.Edges = append(g.Edges, e)
}

// NodeByID returns the node with the given ID, or nil.
func (g *Graph) NodeByID(id string) *Node {
	return g.nodesByID[id]
}

// OutEdges returns all edges originating from the given node ID, sorted by Order.
func (g *Graph) OutEdges(nodeID string) []*Edge {
	var out []*Edge
	for _, e := range g.Edges {
		if e.From == nodeID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Order < out[j].Order
	})
	return out
}

// InEdges returns all edges targeting the given node ID, sorted by Order.
func (g *Graph) InEdges(nodeID string) []*Edge {
	var in []*Edge
	for _, e := range g.Edges {
		if e.To == nodeID {
			in = append(in, e)
		}
	}
	sort.Slice(in, func(i, j int) bool {
		return in[i].Order < in[j].Order
	})
	return in
}

// StartNode returns the node with shape "Mdiamond", or nil.
func (g *Graph) StartNode() *Node {
	for _, n := range g.Nodes {
		if n.Attrs["shape"] == "Mdiamond" {
			return n
		}
	}
	return nil
}

// ExitNodes returns all nodes with shape "Msquare".
func (g *Graph) ExitNodes() []*Node {
	var exits []*Node
	for _, n := range g.Nodes {
		if n.Attrs["shape"] == "Msquare" {
			exits = append(exits, n)
		}
	}
	return exits
}

// Successors returns the IDs of all nodes reachable via outgoing edges from nodeID.
func (g *Graph) Successors(nodeID string) []string {
	var ids []string
	for _, e := range g.OutEdges(nodeID) {
		ids = append(ids, e.To)
	}
	return ids
}

// Predecessors returns the IDs of all nodes with edges targeting nodeID.
func (g *Graph) Predecessors(nodeID string) []string {
	var ids []string
	for _, e := range g.InEdges(nodeID) {
		ids = append(ids, e.From)
	}
	return ids
}

// IntAttr returns the integer value of the named graph attribute, or the default.
func (g *Graph) IntAttr(key string, def int) int {
	v, ok := g.Attrs[key]
	if !ok {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

// BoolAttr returns the boolean value of the named graph attribute, or the default.
func (g *Graph) BoolAttr(key string, def bool) bool {
	v, ok := g.Attrs[key]
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// StringAttr returns the string value of the named graph attribute, or the default.
func (g *Graph) StringAttr(key string, def string) string {
	v, ok := g.Attrs[key]
	if !ok {
		return def
	}
	return v
}

// FloatAttr returns the float64 value of the named graph attribute, or the default.
func (g *Graph) FloatAttr(key string, def float64) float64 {
	v, ok := g.Attrs[key]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// IntAttr returns the integer value of the named attribute, or the default.
func (n *Node) IntAttr(key string, def int) int {
	v, ok := n.Attrs[key]
	if !ok {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

// BoolAttr returns the boolean value of the named attribute, or the default.
func (n *Node) BoolAttr(key string, def bool) bool {
	v, ok := n.Attrs[key]
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// StringAttr returns the string value of the named attribute, or the default.
func (n *Node) StringAttr(key string, def string) string {
	v, ok := n.Attrs[key]
	if !ok {
		return def
	}
	return v
}

// FloatAttr returns the float64 value of the named attribute, or the default.
func (n *Node) FloatAttr(key string, def float64) float64 {
	v, ok := n.Attrs[key]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// IntAttr returns the integer value of the named edge attribute, or the default.
func (e *Edge) IntAttr(key string, def int) int {
	v, ok := e.Attrs[key]
	if !ok {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

// FloatAttr returns the float64 value of the named edge attribute, or the default.
func (e *Edge) FloatAttr(key string, def float64) float64 {
	v, ok := e.Attrs[key]
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// StringAttr returns the string value of the named edge attribute, or the default.
func (e *Edge) StringAttr(key string, def string) string {
	v, ok := e.Attrs[key]
	if !ok {
		return def
	}
	return v
}

// String returns a human-readable representation of a node.
func (n *Node) String() string {
	return fmt.Sprintf("Node(%s)", n.ID)
}

// String returns a human-readable representation of an edge.
func (e *Edge) String() string {
	return fmt.Sprintf("Edge(%s -> %s)", e.From, e.To)
}
