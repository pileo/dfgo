// Package simulate provides a simulation backend for testing Attractor pipelines
// without live LLM API calls. Rules are matched by node ID, node type, or prompt
// regex, with a configurable fallback response.
package simulate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

// Config defines simulation rules and a fallback response.
type Config struct {
	Rules    []Rule `json:"rules"`
	Fallback string `json:"fallback"`
}

// Rule matches pipeline nodes and specifies simulated behavior.
type Rule struct {
	NodeID         string            `json:"node_id,omitempty"`
	NodeType       string            `json:"node_type,omitempty"`
	Pattern        string            `json:"pattern,omitempty"`
	Response       string            `json:"response"`
	Status         string            `json:"status,omitempty"`
	Delay          string            `json:"delay,omitempty"`
	Error          string            `json:"error,omitempty"`
	ContextUpdates map[string]string `json:"context_updates,omitempty"`

	compiledPattern *regexp.Regexp
}

// LoadConfig reads and validates a simulation config from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("simulate: read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("simulate: parse config: %w", err)
	}

	if err := cfg.compile(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// compile pre-compiles regex patterns in rules.
func (c *Config) compile() error {
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Pattern != "" {
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return fmt.Errorf("simulate: invalid regex in rule %d: %w", i, err)
			}
			r.compiledPattern = re
		}
		if r.Delay != "" {
			if _, err := time.ParseDuration(r.Delay); err != nil {
				return fmt.Errorf("simulate: invalid delay in rule %d: %w", i, err)
			}
		}
		if r.Status != "" {
			switch r.Status {
			case "success", "fail", "retry":
			default:
				return fmt.Errorf("simulate: invalid status %q in rule %d (must be success, fail, or retry)", r.Status, i)
			}
		}
	}
	return nil
}

// match finds the first matching rule for the given node ID, node type, and prompt.
// Priority: node_id > node_type > pattern > fallback.
func (c *Config) match(nodeID, nodeType, prompt string) *Rule {
	// Pass 1: match by node ID
	for i := range c.Rules {
		if c.Rules[i].NodeID != "" && c.Rules[i].NodeID == nodeID {
			return &c.Rules[i]
		}
	}
	// Pass 2: match by node type
	for i := range c.Rules {
		if c.Rules[i].NodeType != "" && c.Rules[i].NodeType == nodeType {
			return &c.Rules[i]
		}
	}
	// Pass 3: match by prompt regex
	for i := range c.Rules {
		if c.Rules[i].compiledPattern != nil && c.Rules[i].compiledPattern.MatchString(prompt) {
			return &c.Rules[i]
		}
	}
	return nil
}

// applyDelay sleeps for the rule's configured delay, respecting ctx cancellation.
func applyDelay(ctx context.Context, r *Rule) error {
	if r == nil || r.Delay == "" {
		return nil
	}
	d, _ := time.ParseDuration(r.Delay) // already validated
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Backend implements handler.CodergenBackend for simulation.
type Backend struct {
	cfg *Config
}

// NewBackend creates a simulation CodergenBackend.
func NewBackend(cfg *Config) *Backend {
	return &Backend{cfg: cfg}
}

// Generate returns a simulated response based on matching rules.
func (b *Backend) Generate(ctx context.Context, prompt string, opts map[string]string) (string, error) {
	nodeID := opts["node_id"]
	nodeType := opts["node_type"]
	rule := b.cfg.match(nodeID, nodeType, prompt)

	if err := applyDelay(ctx, rule); err != nil {
		return "", err
	}

	if rule != nil {
		if rule.Status == "fail" || rule.Error != "" {
			errMsg := rule.Error
			if errMsg == "" {
				errMsg = "simulated failure"
			}
			return "", fmt.Errorf("%s", errMsg)
		}
		return rule.Response, nil
	}

	return b.cfg.Fallback, nil
}

// Handler implements handler.Handler for simulation, bypassing the entire LLM path.
type Handler struct {
	cfg *Config
}

// NewHandler creates a simulation Handler.
func NewHandler(cfg *Config) *Handler {
	return &Handler{cfg: cfg}
}

// Execute simulates a pipeline node execution.
func (h *Handler) Execute(ctx context.Context, node *model.Node, pctx *runtime.Context, g *model.Graph, logsDir string) (runtime.Outcome, error) {
	prompt := node.StringAttr("prompt", "")
	nodeType := node.Attrs["type"]
	rule := h.cfg.match(node.ID, nodeType, prompt)

	if err := applyDelay(ctx, rule); err != nil {
		return runtime.Outcome{}, err
	}

	if rule != nil {
		return h.ruleToOutcome(node.ID, rule), nil
	}

	// Fallback
	return runtime.Outcome{
		Status: runtime.StatusSuccess,
		ContextUpdates: map[string]string{
			node.ID + ".response": h.cfg.Fallback,
		},
	}, nil
}

// ruleToOutcome converts a matched rule to a runtime.Outcome.
func (h *Handler) ruleToOutcome(nodeID string, r *Rule) runtime.Outcome {
	updates := map[string]string{
		nodeID + ".response": r.Response,
	}
	for k, v := range r.ContextUpdates {
		updates[k] = v
	}

	status := r.Status
	if status == "" {
		status = "success"
	}

	switch status {
	case "fail":
		errMsg := r.Error
		if errMsg == "" {
			errMsg = "simulated failure"
		}
		o := runtime.FailOutcome(errMsg, runtime.FailureDeterministic)
		o.ContextUpdates = updates
		return o
	case "retry":
		reason := r.Error
		if reason == "" {
			reason = r.Response
		}
		o := runtime.RetryOutcome(reason)
		o.ContextUpdates = updates
		return o
	default: // success
		return runtime.Outcome{
			Status:         runtime.StatusSuccess,
			ContextUpdates: updates,
		}
	}
}

// BuildRegistry creates a handler.Registry with simulation handlers for codergen
// and coding_agent types, plus default handlers for everything else.
func BuildRegistry(cfg *Config) *handler.Registry {
	simHandler := NewHandler(cfg)

	r := handler.DefaultRegistry(
		handler.WithInterviewer(&interviewer.AutoApprove{}),
	)
	r.RegisterType("codergen", simHandler)
	r.RegisterType("coding_agent", simHandler)
	return r
}
