package agent

import (
	"context"
	"fmt"
	"sync"
)

const defaultMaxDepth = 3

// SubagentManager manages child agent sessions spawned from a parent session.
type SubagentManager struct {
	mu       sync.Mutex
	cfg      Config
	agents   map[string]*subagent
	maxDepth int
	depth    int
}

type subagent struct {
	session *Session
	cancel  context.CancelFunc
	done    chan Result
}

// NewSubagentManager creates a manager for spawning child agents.
// depth is the current nesting level (0 = top-level).
func NewSubagentManager(cfg Config, depth int) *SubagentManager {
	return &SubagentManager{
		cfg:      cfg,
		agents:   make(map[string]*subagent),
		maxDepth: defaultMaxDepth,
		depth:    depth,
	}
}

// Spawn creates a new subagent with the given ID and runs it with the input.
// The subagent shares the execution environment but has its own message history.
func (m *SubagentManager) Spawn(ctx context.Context, id string, input string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[id]; exists {
		return fmt.Errorf("subagent %q already exists", id)
	}
	if m.depth >= m.maxDepth {
		return fmt.Errorf("maximum subagent depth (%d) exceeded", m.maxDepth)
	}

	childCfg := m.cfg
	session := NewSession(childCfg)

	childCtx, cancel := context.WithCancel(ctx)
	done := make(chan Result, 1)

	m.agents[id] = &subagent{
		session: session,
		cancel:  cancel,
		done:    done,
	}

	go func() {
		result := session.Run(childCtx, input)
		done <- result
	}()

	return nil
}

// SendInput injects additional input to a running subagent via steering.
func (m *SubagentManager) SendInput(id string, input string) error {
	m.mu.Lock()
	agent, ok := m.agents[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("subagent %q not found", id)
	}
	agent.session.Steer(input)
	return nil
}

// Wait blocks until the subagent completes and returns its result.
func (m *SubagentManager) Wait(id string) (Result, error) {
	m.mu.Lock()
	agent, ok := m.agents[id]
	m.mu.Unlock()

	if !ok {
		return Result{}, fmt.Errorf("subagent %q not found", id)
	}

	result := <-agent.done
	return result, nil
}

// Close cancels and cleans up a subagent.
func (m *SubagentManager) Close(id string) error {
	m.mu.Lock()
	agent, ok := m.agents[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("subagent %q not found", id)
	}
	delete(m.agents, id)
	m.mu.Unlock()

	agent.cancel()
	// Drain result if not already consumed.
	select {
	case <-agent.done:
	default:
	}
	return nil
}

// CloseAll cancels and cleans up all subagents.
func (m *SubagentManager) CloseAll() {
	m.mu.Lock()
	agents := make(map[string]*subagent, len(m.agents))
	for k, v := range m.agents {
		agents[k] = v
	}
	m.agents = make(map[string]*subagent)
	m.mu.Unlock()

	for _, a := range agents {
		a.cancel()
		select {
		case <-a.done:
		default:
		}
	}
}
