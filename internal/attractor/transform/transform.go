// Package transform implements pre-processing transforms applied to prompts and context
// before handler execution.
package transform

import (
	"regexp"
	"strings"
)

// Transform modifies a string (e.g., a prompt) before it's used.
type Transform interface {
	Name() string
	Apply(input string, vars map[string]string) string
}

// Runner applies a sequence of transforms.
type Runner struct {
	Transforms []Transform
}

// NewRunner creates a Runner with the default transforms.
func NewRunner() *Runner {
	return &Runner{
		Transforms: []Transform{
			&VariableExpand{},
		},
	}
}

// Apply runs all transforms in sequence.
func (r *Runner) Apply(input string, vars map[string]string) string {
	for _, t := range r.Transforms {
		input = t.Apply(input, vars)
	}
	return input
}

// VariableExpand replaces $varname and ${varname} references with values from the vars map.
// Special variable: $goal is expanded from the "goal" key.
type VariableExpand struct{}

func (v *VariableExpand) Name() string { return "variable_expand" }

// varPattern matches $identifier or ${identifier} patterns.
var varPattern = regexp.MustCompile(`\$\{([^}]+)\}|\$([a-zA-Z_][a-zA-Z0-9_.]*)\b`)

func (v *VariableExpand) Apply(input string, vars map[string]string) string {
	return varPattern.ReplaceAllStringFunc(input, func(match string) string {
		var key string
		if strings.HasPrefix(match, "${") {
			key = match[2 : len(match)-1]
		} else {
			key = match[1:]
		}
		if val, ok := vars[key]; ok {
			return val
		}
		return match // leave unresolved vars as-is
	})
}
