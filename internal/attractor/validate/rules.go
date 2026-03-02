package validate

import (
	"fmt"

	"dfgo/internal/attractor/cond"
	"dfgo/internal/attractor/model"
)

// BuiltinRules returns all built-in lint rules.
func BuiltinRules() []LintRule {
	return []LintRule{
		&startNodeRule{},
		&terminalNodeRule{},
		&reachabilityRule{},
		&edgeTargetExistsRule{},
		&conditionSyntaxRule{},
		&goalGateRetryRule{},
		&promptOnLLMRule{},
	}
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
				Severity: SeverityWarning,
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
