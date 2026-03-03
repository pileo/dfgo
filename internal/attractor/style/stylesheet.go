// Package style implements a CSS-like model stylesheet parser for Attractor pipelines.
// Stylesheets configure per-node LLM parameters via selectors with specificity.
package style

import (
	"fmt"
	"sort"
	"strings"

	"dfgo/internal/attractor/model"
)

// Specificity: * = 0, .class (shape) = 1, #id = 2
const (
	SpecUniversal = 0
	SpecClass     = 1
	SpecID        = 2
)

// Selector matches nodes by ID, shape, or universally.
type Selector struct {
	Type  string // "*", ".shape", "#id"
	Value string // the shape name or node ID
	Spec  int
}

// Rule maps a selector to a set of properties.
type Rule struct {
	Selector   Selector
	Properties map[string]string
}

// Stylesheet is an ordered list of rules.
type Stylesheet struct {
	Rules []Rule
}

// ParseSelector parses a selector string.
func ParseSelector(s string) Selector {
	s = strings.TrimSpace(s)
	if s == "*" {
		return Selector{Type: "*", Spec: SpecUniversal}
	}
	if strings.HasPrefix(s, "#") {
		return Selector{Type: "#", Value: s[1:], Spec: SpecID}
	}
	if strings.HasPrefix(s, ".") {
		return Selector{Type: ".", Value: s[1:], Spec: SpecClass}
	}
	// Bare identifier = shape class selector
	return Selector{Type: ".", Value: s, Spec: SpecClass}
}

// Matches returns true if the selector matches the given node.
// Class selectors match against both the "shape" attribute and the "class"
// attribute (parsed as a comma-separated list).
func (s Selector) Matches(n *model.Node) bool {
	switch s.Type {
	case "*":
		return true
	case "#":
		return n.ID == s.Value
	case ".":
		if n.Attrs["shape"] == s.Value {
			return true
		}
		for _, cls := range strings.Split(n.Attrs["class"], ",") {
			if strings.TrimSpace(cls) == s.Value {
				return true
			}
		}
		return false
	}
	return false
}

// ParseStylesheet parses a CSS-like stylesheet string.
// Format:
//
//	selector {
//	    property: value;
//	}
//
// Returns an error for structural issues like unclosed braces or empty selectors.
func ParseStylesheet(src string) (Stylesheet, error) {
	var ss Stylesheet
	src = strings.TrimSpace(src)
	for len(src) > 0 {
		// Find selector
		braceIdx := strings.Index(src, "{")
		if braceIdx < 0 {
			// Remaining non-whitespace text without an opening brace
			if strings.TrimSpace(src) != "" {
				return ss, fmt.Errorf("unexpected content without opening brace: %q", strings.TrimSpace(src))
			}
			break
		}
		selectorStr := strings.TrimSpace(src[:braceIdx])
		if selectorStr == "" {
			return ss, fmt.Errorf("empty selector before '{'")
		}
		src = src[braceIdx+1:]

		// Find closing brace
		closeIdx := strings.Index(src, "}")
		if closeIdx < 0 {
			return ss, fmt.Errorf("unclosed brace for selector %q", selectorStr)
		}
		body := src[:closeIdx]
		src = strings.TrimSpace(src[closeIdx+1:])

		sel := ParseSelector(selectorStr)
		props := parseProperties(body)
		if len(props) > 0 {
			ss.Rules = append(ss.Rules, Rule{Selector: sel, Properties: props})
		}
	}
	return ss, nil
}

func parseProperties(body string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(body, ";") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key != "" && val != "" {
			props[key] = val
		}
	}
	return props
}

// Apply applies the stylesheet to all nodes in the graph. Stylesheet properties
// are set on nodes only when the node doesn't already have an explicit value
// for that attribute.
func (ss Stylesheet) Apply(g *model.Graph) {
	for _, n := range g.Nodes {
		props := ss.Resolve(n)
		for k, v := range props {
			if _, exists := n.Attrs[k]; !exists {
				n.Attrs[k] = v
			}
		}
	}
}

// Resolve returns the merged properties for a node, with higher-specificity rules winning.
func (ss Stylesheet) Resolve(n *model.Node) map[string]string {
	type match struct {
		spec  int
		order int
		props map[string]string
	}

	var matches []match
	for i, r := range ss.Rules {
		if r.Selector.Matches(n) {
			matches = append(matches, match{spec: r.Selector.Spec, order: i, props: r.Properties})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].spec != matches[j].spec {
			return matches[i].spec < matches[j].spec
		}
		return matches[i].order < matches[j].order
	})

	result := make(map[string]string)
	for _, m := range matches {
		for k, v := range m.props {
			result[k] = v
		}
	}
	return result
}
