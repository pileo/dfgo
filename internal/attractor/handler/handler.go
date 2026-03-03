// Package handler defines the Handler interface and registry for Attractor pipeline stages.
package handler

import (
	"context"
	"fmt"

	"dfgo/internal/attractor/fidelity"
	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// Handler executes a pipeline stage.
type Handler interface {
	Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error)
}

// FidelityAwareHandler is a handler that can adjust behavior based on fidelity mode.
type FidelityAwareHandler interface {
	Handler
	SetFidelity(mode fidelity.Mode)
}

// SingleExecutionHandler marks handlers that should only execute once even on retry.
type SingleExecutionHandler interface {
	Handler
	IsSingleExecution() bool
}

// Registry maps node shape+type to handlers.
type Registry struct {
	byType  map[string]Handler // "type" attr → handler
	byShape map[string]Handler // "shape" attr → handler (fallback)
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		byType:  make(map[string]Handler),
		byShape: make(map[string]Handler),
	}
}

// RegisterType registers a handler for a node type attribute.
func (r *Registry) RegisterType(nodeType string, h Handler) {
	r.byType[nodeType] = h
}

// RegisterShape registers a handler for a node shape attribute.
func (r *Registry) RegisterShape(shape string, h Handler) {
	r.byShape[shape] = h
}

// KnownTypes returns all registered type attribute values.
func (r *Registry) KnownTypes() []string {
	types := make([]string, 0, len(r.byType))
	for t := range r.byType {
		types = append(types, t)
	}
	return types
}

// KnownShapes returns all registered shape attribute values.
func (r *Registry) KnownShapes() []string {
	shapes := make([]string, 0, len(r.byShape))
	for s := range r.byShape {
		shapes = append(shapes, s)
	}
	return shapes
}

// Lookup finds the handler for a node. Priority: type attr → shape attr.
func (r *Registry) Lookup(n *model.Node) (Handler, error) {
	if t := n.Attrs["type"]; t != "" {
		if h, ok := r.byType[t]; ok {
			return h, nil
		}
	}
	if s := n.Attrs["shape"]; s != "" {
		if h, ok := r.byShape[s]; ok {
			return h, nil
		}
	}
	return nil, fmt.Errorf("no handler for node %q (type=%q, shape=%q)", n.ID, n.Attrs["type"], n.Attrs["shape"])
}

// DefaultRegistry creates a registry with all built-in handlers.
func DefaultRegistry(opts ...RegistryOption) *Registry {
	cfg := registryConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	r := NewRegistry()

	// Shape-based handlers
	r.RegisterShape("Mdiamond", &StartHandler{})
	r.RegisterShape("Msquare", &ExitHandler{})
	r.RegisterShape("diamond", &ConditionalHandler{})

	// Type-based handlers
	r.RegisterType("codergen", NewCodergenHandler(cfg.codergenBackend))
	if cfg.interviewer != nil {
		r.RegisterType("wait.human", NewWaitHumanHandler(cfg.interviewer))
	} else {
		r.RegisterType("wait.human", NewWaitHumanHandler(&interviewer.AutoApprove{}))
	}
	r.RegisterType("conditional", &ConditionalHandler{})
	r.RegisterType("parallel", NewParallelHandler())
	r.RegisterType("parallel.fan_in", &FanInHandler{})
	r.RegisterType("tool", &ToolHandler{})
	r.RegisterType("stack.manager_loop", &ManagerLoopHandler{})
	r.RegisterType("coding_agent", NewCodingAgentHandler(cfg.agentSessionFactory))

	return r
}

type registryConfig struct {
	codergenBackend     CodergenBackend
	interviewer         interviewer.Interviewer
	agentSessionFactory AgentSessionFactory
}

// RegistryOption configures the registry.
type RegistryOption func(*registryConfig)

// WithCodergenBackend sets the LLM backend for codergen handlers.
func WithCodergenBackend(b CodergenBackend) RegistryOption {
	return func(c *registryConfig) { c.codergenBackend = b }
}

// WithInterviewer sets the interviewer for human interaction handlers.
func WithInterviewer(iv interviewer.Interviewer) RegistryOption {
	return func(c *registryConfig) { c.interviewer = iv }
}

// WithAgentSessionFactory sets the factory for creating coding agent sessions.
func WithAgentSessionFactory(f AgentSessionFactory) RegistryOption {
	return func(c *registryConfig) { c.agentSessionFactory = f }
}
