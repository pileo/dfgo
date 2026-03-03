package validate

import (
	"fmt"

	"dfgo/internal/attractor/cond"
	"dfgo/internal/attractor/fidelity"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/style"
)

// BuiltinRules returns all built-in lint rules.
func BuiltinRules(cfg runnerConfig) []LintRule {
	rules := []LintRule{
		&startNodeRule{},
		&startNoIncomingRule{},
		&terminalNodeRule{},
		&exitNoOutgoingRule{},
		&reachabilityRule{},
		&edgeTargetExistsRule{},
		&conditionSyntaxRule{},
		&goalGateRetryRule{},
		&promptOnLLMRule{},
		&stylesheetSyntaxRule{},
		&fidelityValidRule{},
		&retryTargetExistsRule{},
	}
	if len(cfg.knownTypes) > 0 {
		rules = append(rules, &typeKnownRule{knownTypes: cfg.knownTypes})
	}
	return rules
}

// startNodeRule checks that exactly one Mdiamond start node exists.
type startNodeRule struct{}

func (r *startNodeRule) Name() string { return "start_node" }
func (r *startNodeRule) Apply(g *model.Graph) []Diagnostic {
	var starts []*model.Node
	for _, n := range g.Nodes {
		if n.Attrs["shape"] == "Mdiamond" {
			starts = append(starts, n)
		}
	}
	if len(starts) == 0 {
		return []Diagnostic{{Rule: r.Name(), Severity: SeverityError, Message: "no start node (shape=Mdiamond) found"}}
	}
	if len(starts) > 1 {
		return []Diagnostic{{Rule: r.Name(), Severity: SeverityError, Message: fmt.Sprintf("multiple start nodes found: %d", len(starts))}}
	}
	return nil
}

// terminalNodeRule checks that at least one Msquare exit node exists.
type terminalNodeRule struct{}

func (r *terminalNodeRule) Name() string { return "terminal_node" }
func (r *terminalNodeRule) Apply(g *model.Graph) []Diagnostic {
	exits := g.ExitNodes()
	if len(exits) == 0 {
		return []Diagnostic{{Rule: r.Name(), Severity: SeverityError, Message: "no terminal node (shape=Msquare) found"}}
	}
	return nil
}

// reachabilityRule checks that all nodes are reachable from start.
type reachabilityRule struct{}

func (r *reachabilityRule) Name() string { return "reachability" }
func (r *reachabilityRule) Apply(g *model.Graph) []Diagnostic {
	start := g.StartNode()
	if start == nil {
		return nil // start_node rule will catch this
	}

	reachable := make(map[string]bool)
	var walk func(id string)
	walk = func(id string) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		for _, e := range g.OutEdges(id) {
			walk(e.To)
		}
	}
	walk(start.ID)

	var diags []Diagnostic
	for _, n := range g.Nodes {
		if !reachable[n.ID] {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityError,
				NodeID:   n.ID,
				Message:  fmt.Sprintf("node %q is not reachable from start", n.ID),
			})
		}
	}
	return diags
}

// edgeTargetExistsRule checks that all edge targets reference existing nodes.
type edgeTargetExistsRule struct{}

func (r *edgeTargetExistsRule) Name() string { return "edge_target_exists" }
func (r *edgeTargetExistsRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	for _, e := range g.Edges {
		if g.NodeByID(e.From) == nil {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityError,
				Message:  fmt.Sprintf("edge source %q does not exist", e.From),
			})
		}
		if g.NodeByID(e.To) == nil {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityError,
				Message:  fmt.Sprintf("edge target %q does not exist", e.To),
			})
		}
	}
	return diags
}

// conditionSyntaxRule checks that all edge conditions parse correctly.
type conditionSyntaxRule struct{}

func (r *conditionSyntaxRule) Name() string { return "condition_syntax" }
func (r *conditionSyntaxRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	for _, e := range g.Edges {
		condStr := e.Attrs["condition"]
		if condStr == "" {
			continue
		}
		if err := cond.Validate(condStr); err != nil {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityError,
				Message:  fmt.Sprintf("edge %s->%s: invalid condition %q: %v", e.From, e.To, condStr, err),
			})
		}
	}
	return diags
}

// goalGateRetryRule checks that nodes with goal_gate=true have max_retries set.
type goalGateRetryRule struct{}

func (r *goalGateRetryRule) Name() string { return "goal_gate_has_retry" }
func (r *goalGateRetryRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	for _, n := range g.Nodes {
		if n.BoolAttr("goal_gate", false) {
			if _, ok := n.Attrs["max_retries"]; !ok {
				diags = append(diags, Diagnostic{
					Rule:     r.Name(),
					Severity: SeverityWarning,
					NodeID:   n.ID,
					Message:  fmt.Sprintf("node %q has goal_gate=true but no max_retries", n.ID),
				})
			}
		}
	}
	return diags
}

// promptOnLLMRule checks that codergen nodes have a prompt attribute.
type promptOnLLMRule struct{}

func (r *promptOnLLMRule) Name() string { return "prompt_on_llm_nodes" }
func (r *promptOnLLMRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	for _, n := range g.Nodes {
		nodeType := n.Attrs["type"]
		if nodeType == "codergen" {
			if _, ok := n.Attrs["prompt"]; !ok {
				diags = append(diags, Diagnostic{
					Rule:     r.Name(),
					Severity: SeverityWarning,
					NodeID:   n.ID,
					Message:  fmt.Sprintf("node %q is type=codergen but has no prompt attribute", n.ID),
				})
			}
		}
	}
	return diags
}

// startNoIncomingRule checks that the start node has no incoming edges.
type startNoIncomingRule struct{}

func (r *startNoIncomingRule) Name() string { return "start_no_incoming" }
func (r *startNoIncomingRule) Apply(g *model.Graph) []Diagnostic {
	start := g.StartNode()
	if start == nil {
		return nil // start_node rule will catch this
	}
	if in := g.InEdges(start.ID); len(in) > 0 {
		return []Diagnostic{{
			Rule:     r.Name(),
			Severity: SeverityError,
			NodeID:   start.ID,
			Message:  fmt.Sprintf("start node %q has %d incoming edge(s)", start.ID, len(in)),
		}}
	}
	return nil
}

// exitNoOutgoingRule checks that exit nodes have no outgoing edges.
type exitNoOutgoingRule struct{}

func (r *exitNoOutgoingRule) Name() string { return "exit_no_outgoing" }
func (r *exitNoOutgoingRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	for _, n := range g.ExitNodes() {
		if out := g.OutEdges(n.ID); len(out) > 0 {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityError,
				NodeID:   n.ID,
				Message:  fmt.Sprintf("exit node %q has %d outgoing edge(s)", n.ID, len(out)),
			})
		}
	}
	return diags
}

// stylesheetSyntaxRule checks that the model_stylesheet graph attribute parses correctly.
type stylesheetSyntaxRule struct{}

func (r *stylesheetSyntaxRule) Name() string { return "stylesheet_syntax" }
func (r *stylesheetSyntaxRule) Apply(g *model.Graph) []Diagnostic {
	src := g.Attrs["model_stylesheet"]
	if src == "" {
		return nil
	}
	if _, err := style.ParseStylesheet(src); err != nil {
		return []Diagnostic{{
			Rule:     r.Name(),
			Severity: SeverityError,
			Message:  fmt.Sprintf("invalid model_stylesheet: %v", err),
		}}
	}
	return nil
}

// typeKnownRule checks that node type attributes match known handler types.
type typeKnownRule struct {
	knownTypes []string
}

func (r *typeKnownRule) Name() string { return "type_known" }
func (r *typeKnownRule) Apply(g *model.Graph) []Diagnostic {
	known := make(map[string]bool, len(r.knownTypes))
	for _, t := range r.knownTypes {
		known[t] = true
	}
	var diags []Diagnostic
	for _, n := range g.Nodes {
		t := n.Attrs["type"]
		if t == "" {
			continue
		}
		if !known[t] {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityWarning,
				NodeID:   n.ID,
				Message:  fmt.Sprintf("node %q has unknown type %q", n.ID, t),
			})
		}
	}
	return diags
}

// fidelityValidRule checks that fidelity attributes on nodes, edges, and the graph are valid modes.
type fidelityValidRule struct{}

func (r *fidelityValidRule) Name() string { return "fidelity_valid" }
func (r *fidelityValidRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	// Check graph-level fidelity
	if v, ok := g.Attrs["fidelity"]; ok {
		if !fidelity.Mode(v).Valid() {
			diags = append(diags, Diagnostic{
				Rule:     r.Name(),
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("graph has invalid fidelity mode %q", v),
			})
		}
	}
	// Check node-level fidelity
	for _, n := range g.Nodes {
		if v, ok := n.Attrs["fidelity"]; ok {
			if !fidelity.Mode(v).Valid() {
				diags = append(diags, Diagnostic{
					Rule:     r.Name(),
					Severity: SeverityWarning,
					NodeID:   n.ID,
					Message:  fmt.Sprintf("node %q has invalid fidelity mode %q", n.ID, v),
				})
			}
		}
	}
	// Check edge-level fidelity
	for _, e := range g.Edges {
		if v, ok := e.Attrs["fidelity"]; ok {
			if !fidelity.Mode(v).Valid() {
				diags = append(diags, Diagnostic{
					Rule:     r.Name(),
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("edge %s->%s has invalid fidelity mode %q", e.From, e.To, v),
				})
			}
		}
	}
	return diags
}

// retryTargetExistsRule checks that retry_target and fallback_retry_target point to existing nodes.
type retryTargetExistsRule struct{}

func (r *retryTargetExistsRule) Name() string { return "retry_target_exists" }
func (r *retryTargetExistsRule) Apply(g *model.Graph) []Diagnostic {
	var diags []Diagnostic
	for _, n := range g.Nodes {
		for _, attr := range []string{"retry_target", "fallback_retry_target"} {
			target, ok := n.Attrs[attr]
			if !ok || target == "" {
				continue
			}
			if g.NodeByID(target) == nil {
				diags = append(diags, Diagnostic{
					Rule:     r.Name(),
					Severity: SeverityWarning,
					NodeID:   n.ID,
					Message:  fmt.Sprintf("node %q has %s=%q but node %q does not exist", n.ID, attr, target, target),
				})
			}
		}
	}
	return diags
}
