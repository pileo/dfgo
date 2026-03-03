// Package tool defines the Tool interface, Registry, and core tool implementations
// for the coding agent.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dfgo/internal/agent/execenv"
	"dfgo/internal/agent/tool/truncate"
	"dfgo/internal/llm"
)

// Tool represents a tool that the agent can invoke.
type Tool interface {
	// Name returns the tool's identifier.
	Name() string
	// Description returns a human-readable description.
	Description() string
	// Parameters returns the JSON Schema for the tool's arguments.
	Parameters() json.RawMessage
	// Execute runs the tool and returns the result text.
	// Errors are returned as tool results with IsError=true, not as Go errors.
	// Only return a Go error for truly unrecoverable situations.
	Execute(ctx context.Context, env execenv.Environment, args json.RawMessage) (Result, error)
}

// Result holds the output of a tool execution.
type Result struct {
	Content    string // possibly truncated
	FullOutput string // pre-truncation output
	IsError    bool
}

// ErrorResult creates a Result indicating a tool error.
func ErrorResult(msg string) Result {
	return Result{Content: msg, FullOutput: msg, IsError: true}
}

// SuccessResult creates a successful Result, applying truncation.
func SuccessResult(toolName, output string) Result {
	truncated, _ := truncate.Truncate(toolName, output)
	return Result{Content: truncated, FullOutput: output, IsError: false}
}

// Registry manages available tools.
type Registry struct {
	tools map[string]Tool
	order []string // insertion order for deterministic listing
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Lookup finds a tool by name.
func (r *Registry) Lookup(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools in insertion order.
func (r *Registry) All() []Tool {
	tools := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		tools = append(tools, r.tools[name])
	}
	return tools
}

// ToolDefs returns LLM SDK tool definitions for all registered tools.
func (r *Registry) ToolDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// Execute looks up and executes a tool by name.
func (r *Registry) Execute(ctx context.Context, env execenv.Environment, name string, args json.RawMessage) (Result, error) {
	t, ok := r.tools[name]
	if !ok {
		return ErrorResult(fmt.Sprintf("unknown tool: %s", name)), nil
	}
	return t.Execute(ctx, env, args)
}
