package attractor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"dfgo/internal/attractor/dot"
	"dfgo/internal/attractor/edge"
	"dfgo/internal/attractor/fidelity"
	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/rundir"
	"dfgo/internal/attractor/runtime"
	"dfgo/internal/attractor/transform"
	"dfgo/internal/attractor/validate"

	"github.com/google/uuid"
)

// Engine orchestrates the 5-phase Attractor pipeline lifecycle.
type Engine struct {
	Config   EngineConfig
	Registry *handler.Registry
	Graph    *model.Graph
	RunDir   *rundir.RunDir
	PCtx     *runtime.Context
	RunID    string

	checkpoint     *runtime.Checkpoint
	retryCounters  map[string]int
	completed      map[string]bool
	visitLog       []runtime.VisitEntry
	transformRunner *transform.Runner
}

// NewEngine creates a new Engine with the given config.
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		Config:          cfg,
		Registry:        cfg.Registry,
		retryCounters:   make(map[string]int),
		completed:       make(map[string]bool),
		transformRunner: transform.NewRunner(),
	}
}

// Run executes the full 5-phase lifecycle: Parse → Validate → Initialize → Execute → Finalize.
func (e *Engine) Run(ctx context.Context, dotSource string) error {
	// Phase 1: Parse
	g, err := e.parse(dotSource)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	e.Graph = g

	// Phase 2: Validate
	if err := e.validate(g); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Phase 3: Initialize
	if err := e.initialize(g); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Phase 4: Execute
	if err := e.execute(ctx, g); err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	// Phase 5: Finalize
	return e.finalize()
}

func (e *Engine) parse(dotSource string) (*model.Graph, error) {
	slog.Info("parsing pipeline")
	return dot.Parse(dotSource)
}

func (e *Engine) validate(g *model.Graph) error {
	slog.Info("validating pipeline")
	runner := validate.NewRunner()
	diags := runner.Run(g)

	for _, d := range diags {
		if d.Severity == validate.SeverityError {
			slog.Error("validation error", "rule", d.Rule, "message", d.Message, "node", d.NodeID)
		} else {
			slog.Warn("validation warning", "rule", d.Rule, "message", d.Message, "node", d.NodeID)
		}
	}

	if validate.HasErrors(diags) {
		return fmt.Errorf("pipeline has %d validation errors", len(validate.Errors(diags)))
	}
	return nil
}

func (e *Engine) initialize(g *model.Graph) error {
	// Generate run ID
	if e.Config.ResumeRunID != "" {
		e.RunID = e.Config.ResumeRunID
	} else {
		e.RunID = uuid.New().String()
	}
	slog.Info("initializing run", "run_id", e.RunID)

	// Create run directory
	logsRoot := e.Config.LogsDir
	if logsRoot == "" {
		logsRoot = "runs"
	}
	rd, err := rundir.Create(logsRoot, e.RunID)
	if err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	e.RunDir = rd

	// Initialize context
	e.PCtx = runtime.NewContext()
	if goal := g.Attrs["goal"]; goal != "" {
		e.PCtx.Set("goal", goal)
	}
	for k, v := range e.Config.InitialContext {
		e.PCtx.Set(k, v)
	}

	// Write initial manifest
	if err := rd.WriteManifest(rundir.Manifest{
		RunID:     e.RunID,
		Pipeline:  g.Name,
		StartedAt: time.Now(),
		Status:    "running",
		NodeCount: len(g.Nodes),
	}); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Resume from checkpoint if available
	if e.Config.ResumeRunID != "" {
		cp, err := runtime.LoadCheckpoint(rd.CheckpointPath())
		if err != nil {
			slog.Warn("could not load checkpoint, starting fresh", "error", err)
		} else {
			e.checkpoint = cp
			e.PCtx.Merge(cp.Context)
			e.retryCounters = cp.RetryCounters
			for _, id := range cp.Completed {
				e.completed[id] = true
			}
			e.visitLog = cp.VisitLog
			slog.Info("resumed from checkpoint", "current_node", cp.CurrentNode, "completed", len(cp.Completed))
		}
	}

	return nil
}

func (e *Engine) execute(ctx context.Context, g *model.Graph) error {
	start := g.StartNode()
	if start == nil {
		return fmt.Errorf("no start node")
	}

	// Determine starting node
	currentID := start.ID
	if e.checkpoint != nil && e.checkpoint.CurrentNode != "" {
		currentID = e.checkpoint.CurrentNode
	}

	slog.Info("starting execution", "node", currentID)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node := g.NodeByID(currentID)
		if node == nil {
			return fmt.Errorf("node %q not found", currentID)
		}

		// Check if this is an exit node
		if node.Attrs["shape"] == "Msquare" {
			slog.Info("reached exit node", "node", currentID)
			e.completed[currentID] = true
			e.visitLog = append(e.visitLog, runtime.VisitEntry{
				NodeID: currentID, Status: runtime.StatusSuccess, Attempt: 1,
			})
			break
		}

		// Execute the node
		outcome, err := e.executeNode(ctx, g, node)
		if err != nil {
			return fmt.Errorf("execute node %q: %w", currentID, err)
		}

		slog.Info("node completed", "node", currentID, "status", outcome.Status, "label", outcome.PreferredLabel)

		// Apply context updates
		if outcome.ContextUpdates != nil {
			e.PCtx.Merge(outcome.ContextUpdates)
		}

		// Record visit
		attempt := e.retryCounters[currentID] + 1
		e.visitLog = append(e.visitLog, runtime.VisitEntry{
			NodeID: currentID, Status: outcome.Status, Attempt: attempt,
		})

		// Handle retry
		if outcome.Status == runtime.StatusRetry {
			maxRetries := node.IntAttr("max_retries", 0)
			if e.retryCounters[currentID] < maxRetries {
				e.retryCounters[currentID]++
				slog.Info("retrying node", "node", currentID, "attempt", e.retryCounters[currentID], "max", maxRetries)
				e.saveCheckpoint(currentID)
				continue
			}
			slog.Warn("max retries exceeded", "node", currentID)
			outcome.Status = runtime.StatusFail
			outcome.FailureReason = "max retries exceeded"
		}

		// Handle failure with goal gate
		if outcome.Status == runtime.StatusFail && node.BoolAttr("goal_gate", false) {
			maxRetries := node.IntAttr("max_retries", 0)
			if e.retryCounters[currentID] < maxRetries {
				e.retryCounters[currentID]++
				slog.Info("goal gate retry", "node", currentID, "attempt", e.retryCounters[currentID])
				e.saveCheckpoint(currentID)
				continue
			}
			return fmt.Errorf("goal gate failed at node %q after %d retries: %s",
				currentID, maxRetries, outcome.FailureReason)
		}

		e.completed[currentID] = true

		// Select next edge
		nextEdge := edge.Select(g, currentID, outcome, e.PCtx)
		if nextEdge == nil {
			// No outgoing edge — check if this is a terminal
			if len(g.OutEdges(currentID)) == 0 {
				slog.Info("node has no outgoing edges, stopping", "node", currentID)
				break
			}
			return fmt.Errorf("no matching edge from node %q", currentID)
		}

		slog.Info("transitioning", "from", currentID, "to", nextEdge.To, "label", nextEdge.Attrs["label"])
		e.saveCheckpoint(nextEdge.To)
		currentID = nextEdge.To
	}

	return nil
}

func (e *Engine) executeNode(ctx context.Context, g *model.Graph, node *model.Node) (runtime.Outcome, error) {
	h, err := e.Registry.Lookup(node)
	if err != nil {
		return runtime.Outcome{}, err
	}

	// Apply fidelity if handler supports it
	if fh, ok := h.(handler.FidelityAwareHandler); ok {
		mode := fidelity.Resolve(nil, node, g)
		fh.SetFidelity(mode)
	}

	// Prepare logs directory
	logsDir := ""
	if e.RunDir != nil {
		logsDir, _ = e.RunDir.NodeDir(node.ID)
	}

	return h.Execute(ctx, node, e.PCtx, g, logsDir)
}

func (e *Engine) saveCheckpoint(currentNode string) {
	if e.RunDir == nil {
		return
	}
	var completedList []string
	for id := range e.completed {
		completedList = append(completedList, id)
	}
	cp := &runtime.Checkpoint{
		RunID:         e.RunID,
		CurrentNode:   currentNode,
		Completed:     completedList,
		RetryCounters: e.retryCounters,
		Context:       e.PCtx.Snapshot(),
		VisitLog:      e.visitLog,
	}
	if err := cp.Save(e.RunDir.CheckpointPath()); err != nil {
		slog.Error("failed to save checkpoint", "error", err)
	}
}

func (e *Engine) finalize() error {
	slog.Info("finalizing run", "run_id", e.RunID)

	// Save final checkpoint
	e.saveCheckpoint("")

	// Update manifest
	if e.RunDir != nil {
		return e.RunDir.WriteManifest(rundir.Manifest{
			RunID:     e.RunID,
			Pipeline:  e.Graph.Name,
			StartedAt: time.Now(),
			Status:    "completed",
			NodeCount: len(e.Graph.Nodes),
		})
	}
	return nil
}
