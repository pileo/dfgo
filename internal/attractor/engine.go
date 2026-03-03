package attractor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"dfgo/internal/attractor/artifact"
	"dfgo/internal/attractor/dot"
	"dfgo/internal/attractor/edge"
	"dfgo/internal/attractor/events"
	"dfgo/internal/attractor/fidelity"
	"dfgo/internal/attractor/style"
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
	Config    EngineConfig
	Registry  *handler.Registry
	Graph     *model.Graph
	RunDir    *rundir.RunDir
	PCtx      *runtime.Context
	RunID     string
	Artifacts *artifact.Store
	Events    *events.Emitter

	checkpoint      *runtime.Checkpoint
	retryCounters   map[string]int
	completed       map[string]bool
	visitLog        []runtime.VisitEntry
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

	// Apply stylesheet transform (between parse and validate)
	if err := e.applyStylesheet(g); err != nil {
		return fmt.Errorf("stylesheet: %w", err)
	}

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
		if e.Events != nil {
			e.Events.Emit(events.PipelineFailed, map[string]any{
				"run_id": e.RunID,
				"error":  err.Error(),
			})
			e.Events.Close()
		}
		return fmt.Errorf("execute: %w", err)
	}

	// Phase 5: Finalize
	return e.finalize()
}

// nodeStatus is the JSON structure written to each node's status.json.
type nodeStatus struct {
	Outcome            string            `json:"outcome"`
	PreferredNextLabel string            `json:"preferred_next_label"`
	SuggestedNextIDs   []string          `json:"suggested_next_ids"`
	ContextUpdates     map[string]string `json:"context_updates"`
	Notes              string            `json:"notes"`
}

// writeNodeStatus writes a status.json file to the node's log directory.
func (e *Engine) writeNodeStatus(node *model.Node, outcome runtime.Outcome) {
	if e.RunDir == nil {
		return
	}
	dir, err := e.RunDir.NodeDir(node.ID)
	if err != nil {
		slog.Error("failed to create node dir for status", "node", node.ID, "error", err)
		return
	}
	nextIDs := outcome.SuggestedNextIDs
	if nextIDs == nil {
		nextIDs = []string{}
	}
	updates := outcome.ContextUpdates
	if updates == nil {
		updates = map[string]string{}
	}
	status := nodeStatus{
		Outcome:            string(outcome.Status),
		PreferredNextLabel: outcome.PreferredLabel,
		SuggestedNextIDs:   nextIDs,
		ContextUpdates:     updates,
		Notes:              outcome.Notes,
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		slog.Error("failed to marshal node status", "node", node.ID, "error", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644); err != nil {
		slog.Error("failed to write node status", "node", node.ID, "error", err)
	}
}

func (e *Engine) applyStylesheet(g *model.Graph) error {
	src, ok := g.Attrs["model_stylesheet"]
	if !ok || src == "" {
		return nil
	}
	ss, err := style.ParseStylesheet(src)
	if err != nil {
		return fmt.Errorf("parse stylesheet: %w", err)
	}
	ss.Apply(g)
	slog.Info("applied stylesheet", "rules", len(ss.Rules))
	return nil
}

func (e *Engine) parse(dotSource string) (*model.Graph, error) {
	slog.Info("parsing pipeline")
	return dot.Parse(dotSource)
}

func (e *Engine) validate(g *model.Graph) error {
	slog.Info("validating pipeline")
	var opts []validate.RunnerOption
	if e.Registry != nil {
		opts = append(opts, validate.WithKnownTypes(e.Registry.KnownTypes()))
	}
	runner := validate.NewRunner(opts...)
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

	// Initialize event emitter
	e.Events = events.NewEmitter(256)

	// Initialize artifact store
	if e.Config.Artifacts != nil {
		e.Artifacts = e.Config.Artifacts
	} else {
		e.Artifacts = artifact.NewStore(rd.ArtifactsDir())
	}

	// Initialize context
	e.PCtx = runtime.NewContext()
	if goal := g.Attrs["goal"]; goal != "" {
		e.PCtx.Set("goal", goal)
		e.PCtx.Set("graph.goal", goal)
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
			for _, entry := range cp.Logs {
				e.PCtx.AppendLog(entry)
			}
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
	e.Events.Emit(events.PipelineStarted, map[string]any{
		"run_id":   e.RunID,
		"pipeline": g.Name,
		"start":    currentID,
	})

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
			// Check goal gates before accepting exit
			if gateID := e.checkGoalGates(g); gateID != "" {
				gateNode := g.NodeByID(gateID)
				maxRetries := gateNode.IntAttr("max_retries", g.IntAttr("default_max_retry", 50))
				if e.retryCounters[gateID] < maxRetries {
					e.retryCounters[gateID]++
					e.PCtx.Set("internal.retry_count."+gateID, strconv.Itoa(e.retryCounters[gateID]))
					slog.Info("goal gate unsatisfied at exit, retrying", "gate", gateID, "attempt", e.retryCounters[gateID])
					e.saveCheckpoint(gateID)
					if target, ok := e.resolveRetryTarget(g, gateID); ok {
						currentID = target
						continue
					}
					currentID = gateID
					continue
				}
				return fmt.Errorf("goal gate %q unsatisfied at exit after %d retries", gateID, maxRetries)
			}
			slog.Info("reached exit node", "node", currentID)
			e.completed[currentID] = true
			e.visitLog = append(e.visitLog, runtime.VisitEntry{
				NodeID: currentID, Status: runtime.StatusSuccess, Attempt: 1,
			})
			break
		}

		// Set current_node before execution
		e.PCtx.Set("current_node", currentID)
		e.Events.Emit(events.StageStarted, map[string]any{
			"node_id": currentID,
			"type":    node.StringAttr("type", ""),
			"shape":   node.StringAttr("shape", ""),
		})

		// Execute the node
		outcome, err := e.executeNode(ctx, g, node)
		if err != nil {
			e.Events.Emit(events.StageFailed, map[string]any{
				"node_id": currentID,
				"error":   err.Error(),
			})
			return fmt.Errorf("execute node %q: %w", currentID, err)
		}

		if outcome.IsSuccess() {
			e.Events.Emit(events.StageCompleted, map[string]any{
				"node_id": currentID,
				"status":  string(outcome.Status),
			})
		} else if outcome.Status == runtime.StatusFail {
			e.Events.Emit(events.StageFailed, map[string]any{
				"node_id": currentID,
				"status":  string(outcome.Status),
				"reason":  outcome.FailureReason,
			})
		}

		// Write per-node status.json
		e.writeNodeStatus(node, outcome)

		slog.Info("node completed", "node", currentID, "status", outcome.Status, "label", outcome.PreferredLabel)

		// Apply context updates
		if outcome.ContextUpdates != nil {
			e.PCtx.Merge(outcome.ContextUpdates)
		}
		e.PCtx.Set("outcome", string(outcome.Status))
		if outcome.PreferredLabel != "" {
			e.PCtx.Set("preferred_label", outcome.PreferredLabel)
		}

		// Record visit
		attempt := e.retryCounters[currentID] + 1
		e.visitLog = append(e.visitLog, runtime.VisitEntry{
			NodeID: currentID, Status: outcome.Status, Attempt: attempt,
		})

		// Handle retry
		if outcome.Status == runtime.StatusRetry {
			maxRetries := node.IntAttr("max_retries", g.IntAttr("default_max_retry", 50))
			if e.retryCounters[currentID] < maxRetries {
				e.retryCounters[currentID]++
				e.PCtx.Set("internal.retry_count."+currentID, strconv.Itoa(e.retryCounters[currentID]))
				slog.Info("retrying node", "node", currentID, "attempt", e.retryCounters[currentID], "max", maxRetries)
				e.Events.Emit(events.StageRetrying, map[string]any{
					"node_id": currentID,
					"attempt": e.retryCounters[currentID],
					"max":     maxRetries,
				})
				e.saveCheckpoint(currentID)
				policy := runtime.PolicyByName(node.StringAttr("retry_policy", "standard"))
				delay := policy.DelayForAttempt(e.retryCounters[currentID])
				if delay > 0 {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				continue
			}
			slog.Warn("max retries exceeded", "node", currentID)
			if node.BoolAttr("allow_partial", false) {
				outcome.Status = runtime.StatusPartialSuccess
				outcome.Notes = "retries exhausted, partial accepted"
			} else {
				outcome.Status = runtime.StatusFail
				outcome.FailureReason = "max retries exceeded"
			}
		}

		// Handle failure with goal gate
		if outcome.Status == runtime.StatusFail && node.BoolAttr("goal_gate", false) {
			maxRetries := node.IntAttr("max_retries", g.IntAttr("default_max_retry", 50))
			if e.retryCounters[currentID] < maxRetries {
				e.retryCounters[currentID]++
				e.PCtx.Set("internal.retry_count."+currentID, strconv.Itoa(e.retryCounters[currentID]))
				slog.Info("goal gate retry", "node", currentID, "attempt", e.retryCounters[currentID])
				e.saveCheckpoint(currentID)
				policy := runtime.PolicyByName(node.StringAttr("retry_policy", "standard"))
				delay := policy.DelayForAttempt(e.retryCounters[currentID])
				if delay > 0 {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				continue
			}
			return fmt.Errorf("goal gate failed at node %q after %d retries: %s",
				currentID, maxRetries, outcome.FailureReason)
		}

		e.completed[currentID] = true

		// Parallel fan-out: children already executed by handler, skip to convergence.
		if node.Attrs["type"] == "parallel" {
			if nextID, ok := e.handleParallelComplete(g, currentID); ok {
				slog.Info("parallel complete, jumping to convergence", "from", currentID, "to", nextID)
				e.saveCheckpoint(nextID)
				currentID = nextID
				continue
			}
		}

		// Select next edge
		nextEdge := edge.Select(g, currentID, outcome, e.PCtx)
		if nextEdge == nil {
			// No outgoing edge — check if this is a terminal
			if len(g.OutEdges(currentID)) == 0 {
				slog.Info("node has no outgoing edges, stopping", "node", currentID)
				break
			}
			// Try retry_target chain as fallback for failed edge selection
			if !outcome.IsSuccess() {
				if target, ok := e.resolveRetryTarget(g, currentID); ok {
					slog.Info("no matching edge, following retry_target", "from", currentID, "to", target)
					currentID = target
					continue
				}
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

	// Wire ChildExecutor for parallel handlers so they execute children through the engine.
	if ph, ok := h.(*handler.ParallelHandler); ok {
		ph.ChildExecutor = func(childCtx context.Context, childNodeID string) (runtime.Outcome, error) {
			childNode := g.NodeByID(childNodeID)
			if childNode == nil {
				return runtime.Outcome{}, fmt.Errorf("parallel child %q not found", childNodeID)
			}
			return e.executeNode(childCtx, g, childNode)
		}
	}

	// Resolve fidelity and generate preamble
	mode := fidelity.Resolve(nil, node, g)
	if fh, ok := h.(handler.FidelityAwareHandler); ok {
		fh.SetFidelity(mode)
	}
	goal, _ := e.PCtx.Get("goal")
	preamble := fidelity.GeneratePreamble(mode, e.RunID, goal, e.visitLog)
	if preamble != "" {
		e.PCtx.Set("internal.preamble", preamble)
	}

	// Prepare logs directory
	logsDir := ""
	if e.RunDir != nil {
		logsDir, _ = e.RunDir.NodeDir(node.ID)
	}

	return h.Execute(ctx, node, e.PCtx, g, logsDir)
}

// checkGoalGates returns the ID of the first unsatisfied goal_gate node, or "".
func (e *Engine) checkGoalGates(g *model.Graph) string {
	// Build latest-status map from visitLog (last visit wins)
	latestStatus := make(map[string]runtime.StageStatus)
	for _, v := range e.visitLog {
		latestStatus[v.NodeID] = v.Status
	}
	for _, n := range g.Nodes {
		if !n.BoolAttr("goal_gate", false) {
			continue
		}
		status, visited := latestStatus[n.ID]
		if !visited {
			return n.ID
		}
		if status != runtime.StatusSuccess && status != runtime.StatusPartialSuccess {
			return n.ID
		}
	}
	return ""
}

// resolveRetryTarget walks the retry target chain for a node.
// Priority: node.retry_target → node.fallback_retry_target →
// graph.retry_target → graph.fallback_retry_target.
func (e *Engine) resolveRetryTarget(g *model.Graph, nodeID string) (string, bool) {
	node := g.NodeByID(nodeID)
	if node == nil {
		return "", false
	}
	candidates := []string{
		node.StringAttr("retry_target", ""),
		node.StringAttr("fallback_retry_target", ""),
		g.StringAttr("retry_target", ""),
		g.StringAttr("fallback_retry_target", ""),
	}
	for _, c := range candidates {
		if c != "" && g.NodeByID(c) != nil {
			return c, true
		}
	}
	return "", false
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
		Logs:          e.PCtx.Logs(),
		VisitLog:      e.visitLog,
	}
	if err := cp.Save(e.RunDir.CheckpointPath()); err != nil {
		slog.Error("failed to save checkpoint", "error", err)
	} else if e.Events != nil {
		e.Events.Emit(events.CheckpointSaved, map[string]any{
			"current_node": currentNode,
		})
	}
}

// handleParallelComplete processes results from a completed parallel fan-out.
// It marks children as completed, applies their context updates, and returns
// the convergence node ID (typically a fan_in).
func (e *Engine) handleParallelComplete(g *model.Graph, fanOutID string) (string, bool) {
	resultsJSON, ok := e.PCtx.Get("parallel.results")
	if !ok {
		return "", false
	}

	var childOutcomes map[string]runtime.Outcome
	if err := json.Unmarshal([]byte(resultsJSON), &childOutcomes); err != nil {
		return "", false
	}

	// Apply context updates and mark children completed.
	for childID, childOutcome := range childOutcomes {
		if childOutcome.ContextUpdates != nil {
			e.PCtx.Merge(childOutcome.ContextUpdates)
		}
		e.completed[childID] = true
		if childNode := g.NodeByID(childID); childNode != nil {
			e.writeNodeStatus(childNode, childOutcome)
		}
		e.visitLog = append(e.visitLog, runtime.VisitEntry{
			NodeID: childID, Status: childOutcome.Status, Attempt: 1,
		})
		e.Events.Emit(events.StageCompleted, map[string]any{
			"node_id": childID,
			"status":  string(childOutcome.Status),
		})
	}

	// Find convergence: common successor of all children.
	children := g.Successors(fanOutID)
	return findConvergence(g, children)
}

// findConvergence returns the first node (by declaration order) that is a
// direct successor of every child. This is typically the fan_in node.
func findConvergence(g *model.Graph, children []string) (string, bool) {
	if len(children) == 0 {
		return "", false
	}

	// Build successor sets for each child.
	sets := make([]map[string]bool, len(children))
	for i, childID := range children {
		sets[i] = make(map[string]bool)
		for _, s := range g.Successors(childID) {
			sets[i][s] = true
		}
	}

	// Find first node (by declaration order) present in all successor sets.
	for _, n := range g.Nodes {
		inAll := true
		for _, set := range sets {
			if !set[n.ID] {
				inAll = false
				break
			}
		}
		if inAll {
			return n.ID, true
		}
	}
	return "", false
}

func (e *Engine) finalize() error {
	slog.Info("finalizing run", "run_id", e.RunID)

	// Save final checkpoint
	e.saveCheckpoint("")

	e.Events.Emit(events.PipelineCompleted, map[string]any{
		"run_id":   e.RunID,
		"pipeline": e.Graph.Name,
	})
	e.Events.Close()

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
