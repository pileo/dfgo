// Package runmgr tracks concurrent pipeline runs for the HTTP server.
package runmgr

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"dfgo/internal/attractor"
	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/server/sse"

	"github.com/google/uuid"
)

// RunStatus represents the lifecycle state of a pipeline run.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
	StatusCanceled  RunStatus = "canceled"
)

// Run tracks a single pipeline execution.
type Run struct {
	ID          string
	Status      RunStatus
	Pipeline    string
	StartedAt   time.Time
	CompletedAt time.Time
	Error       string

	engine      *attractor.Engine
	cancel      context.CancelFunc
	broadcaster *sse.Broadcaster
	interviewer *interviewer.HTTP
	wg          sync.WaitGroup
	mu          sync.RWMutex
}

// Broadcaster returns the run's SSE broadcaster.
func (r *Run) Broadcaster() *sse.Broadcaster { return r.broadcaster }

// Interviewer returns the run's HTTP interviewer (nil if auto-approve).
func (r *Run) Interviewer() *interviewer.HTTP { return r.interviewer }

// Engine returns the run's engine.
func (r *Run) Engine() *attractor.Engine { return r.engine }

func (r *Run) setStatus(s RunStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = s
}

func (r *Run) setCompleted(s RunStatus, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = s
	r.CompletedAt = time.Now()
	r.Error = errMsg
}

// Snapshot returns a copy of the run's public fields, safe for concurrent reads.
func (r *Run) Snapshot() Run {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return Run{
		ID:          r.ID,
		Status:      r.Status,
		Pipeline:    r.Pipeline,
		StartedAt:   r.StartedAt,
		CompletedAt: r.CompletedAt,
		Error:       r.Error,
	}
}

// ManagerConfig configures the RunManager.
type ManagerConfig struct {
	// BaseCfg provides defaults for engine configuration.
	BaseCfg attractor.EngineConfig
}

// RunManager tracks concurrent pipeline runs.
type RunManager struct {
	mu   sync.RWMutex
	runs map[string]*Run
	cfg  ManagerConfig
}

// NewRunManager creates a new RunManager.
func NewRunManager(cfg ManagerConfig) *RunManager {
	return &RunManager{
		runs: make(map[string]*Run),
		cfg:  cfg,
	}
}

// SubmitOptions configures a single pipeline submission.
type SubmitOptions struct {
	DOTSource      string
	InitialContext map[string]string
	AutoApprove    bool
}

// Submit starts a new pipeline run. Returns the run ID immediately.
// The pipeline is prepared synchronously (parse+validate), and execution
// is launched in a background goroutine.
func (m *RunManager) Submit(ctx context.Context, opts SubmitOptions) (string, error) {
	runID := uuid.New().String()

	// Use a detached context for the run's lifetime — the HTTP request context
	// only gates the synchronous Prepare() call below.
	runCtx, cancel := context.WithCancel(context.Background())
	bc := sse.NewBroadcaster()

	// Build engine config from base + submit options.
	cfg := m.cfg.BaseCfg
	cfg.InitialContext = opts.InitialContext

	// Set up interviewer.
	var httpIV *interviewer.HTTP
	if opts.AutoApprove || cfg.AutoApprove {
		cfg.Interviewer = &interviewer.AutoApprove{}
	} else {
		httpIV = interviewer.NewHTTP(runCtx)
		cfg.Interviewer = httpIV
	}

	// Build handler registry.
	var regOpts []handler.RegistryOption
	if cfg.CodergenBackend != nil {
		regOpts = append(regOpts, handler.WithCodergenBackend(cfg.CodergenBackend))
	}
	if cfg.Interviewer != nil {
		regOpts = append(regOpts, handler.WithInterviewer(cfg.Interviewer))
	}
	if cfg.AgentSessionFactory != nil {
		regOpts = append(regOpts, handler.WithAgentSessionFactory(cfg.AgentSessionFactory))
	}
	cfg.Registry = handler.DefaultRegistry(regOpts...)

	engine := attractor.NewEngine(cfg)

	// Prepare synchronously — validates the DOT source early.
	if err := engine.Prepare(runCtx, opts.DOTSource); err != nil {
		cancel()
		bc.Close()
		return "", fmt.Errorf("prepare: %w", err)
	}

	// Wire broadcaster to engine events.
	engine.Events.On(bc.Publish)

	pipelineName := ""
	if engine.Graph != nil {
		pipelineName = engine.Graph.Name
	}

	run := &Run{
		ID:          runID,
		Status:      StatusRunning,
		Pipeline:    pipelineName,
		StartedAt:   time.Now(),
		engine:      engine,
		cancel:      cancel,
		broadcaster: bc,
		interviewer: httpIV,
	}

	m.mu.Lock()
	m.runs[runID] = run
	m.mu.Unlock()

	// Launch execution in background.
	run.wg.Add(1)
	go func() {
		defer run.wg.Done()
		defer bc.Close()
		defer cancel()

		if err := engine.Execute(runCtx); err != nil {
			if runCtx.Err() != nil {
				run.setCompleted(StatusCanceled, "canceled")
				slog.Info("run canceled", "run_id", runID)
			} else {
				run.setCompleted(StatusFailed, err.Error())
				slog.Error("run failed", "run_id", runID, "error", err)
			}
			return
		}
		run.setCompleted(StatusCompleted, "")
		slog.Info("run completed", "run_id", runID)
	}()

	return runID, nil
}

// Get returns a run by ID, or nil if not found.
func (m *RunManager) Get(id string) *Run {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runs[id]
}

// Cancel cancels a running pipeline.
func (m *RunManager) Cancel(id string) error {
	r := m.Get(id)
	if r == nil {
		return fmt.Errorf("run %q not found", id)
	}
	r.cancel()
	return nil
}

// Shutdown cancels all running pipelines and waits for them to finish.
func (m *RunManager) Shutdown(ctx context.Context) {
	m.mu.RLock()
	runs := make([]*Run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.RUnlock()

	for _, r := range runs {
		r.cancel()
	}

	done := make(chan struct{})
	go func() {
		for _, r := range runs {
			r.wg.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}
