// Package validate implements lint-style validation for Attractor pipeline graphs.
package validate

import (
	"dfgo/internal/attractor/model"
)

// Severity indicates how serious a diagnostic is.
type Severity int

const (
	SeverityError   Severity = iota // blocks execution
	SeverityWarning                 // informational, does not block
)

func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// Diagnostic is a single validation finding.
type Diagnostic struct {
	Rule     string
	Severity Severity
	NodeID   string // optional, if the finding relates to a specific node
	Message  string
}

// LintRule is a validation rule that checks a graph.
type LintRule interface {
	Name() string
	Apply(g *model.Graph) []Diagnostic
}

// Runner executes a set of lint rules against a graph.
type Runner struct {
	Rules []LintRule
}

// RunnerOption configures a validation Runner.
type RunnerOption func(*runnerConfig)

type runnerConfig struct {
	knownTypes []string
}

// WithKnownTypes provides the set of known node type values for the type_known rule.
func WithKnownTypes(types []string) RunnerOption {
	return func(c *runnerConfig) { c.knownTypes = types }
}

// NewRunner creates a Runner with the default set of built-in rules.
func NewRunner(opts ...RunnerOption) *Runner {
	cfg := runnerConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return &Runner{
		Rules: BuiltinRules(cfg),
	}
}

// Run applies all rules and returns all diagnostics.
func (r *Runner) Run(g *model.Graph) []Diagnostic {
	var all []Diagnostic
	for _, rule := range r.Rules {
		all = append(all, rule.Apply(g)...)
	}
	return all
}

// HasErrors returns true if any diagnostic is an error.
func HasErrors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only error-severity diagnostics.
func Errors(diags []Diagnostic) []Diagnostic {
	var errs []Diagnostic
	for _, d := range diags {
		if d.Severity == SeverityError {
			errs = append(errs, d)
		}
	}
	return errs
}
