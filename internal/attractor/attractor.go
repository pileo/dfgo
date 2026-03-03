// Package attractor implements the Attractor pipeline orchestration engine.
// Pipelines are declared as Graphviz DOT graphs where nodes are stages
// (LLM calls, human approvals, tools) and edges define conditional transitions.
package attractor

import (
	"context"

	"dfgo/internal/attractor/artifact"
	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/interviewer"
)

// EngineConfig configures the Attractor engine.
type EngineConfig struct {
	// Registry maps node types/shapes to handlers.
	// If nil, DefaultRegistry is used.
	Registry *handler.Registry

	// LogsDir is the root directory for run logs. Defaults to "runs".
	LogsDir string

	// ResumeRunID, if set, resumes a previous run by loading its checkpoint.
	ResumeRunID string

	// InitialContext provides initial key-value pairs for the pipeline context.
	InitialContext map[string]string

	// CodergenBackend is the LLM backend for codergen stages.
	CodergenBackend handler.CodergenBackend

	// AgentSessionFactory creates coding agent sessions for coding_agent stages.
	AgentSessionFactory handler.AgentSessionFactory

	// Interviewer is used for human interaction stages.
	Interviewer interviewer.Interviewer

	// AutoApprove makes the engine auto-approve all human prompts.
	AutoApprove bool

	// Artifacts is an optional artifact store for the pipeline.
	// If nil, one is created automatically using the run directory.
	Artifacts *artifact.Store

	// CXDBAddr is the CXDB binary protocol address (e.g., "localhost:9009").
	// Empty string disables CXDB recording.
	CXDBAddr string
}

// RunPipeline is a convenience function that creates an engine and runs a pipeline.
func RunPipeline(ctx context.Context, dotSource string, cfg EngineConfig) error {
	if cfg.Registry == nil {
		var opts []handler.RegistryOption
		if cfg.CodergenBackend != nil {
			opts = append(opts, handler.WithCodergenBackend(cfg.CodergenBackend))
		}
		iv := cfg.Interviewer
		if iv == nil && cfg.AutoApprove {
			iv = &interviewer.AutoApprove{}
		}
		if iv != nil {
			opts = append(opts, handler.WithInterviewer(iv))
		}
		if cfg.AgentSessionFactory != nil {
			opts = append(opts, handler.WithAgentSessionFactory(cfg.AgentSessionFactory))
		}
		cfg.Registry = handler.DefaultRegistry(opts...)
	}

	engine := NewEngine(cfg)
	return engine.Run(ctx, dotSource)
}
