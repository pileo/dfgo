package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Checkpoint captures the full execution state for resumption.
type Checkpoint struct {
	RunID         string            `json:"run_id"`
	CurrentNode   string            `json:"current_node"`
	Completed     []string          `json:"completed"`
	RetryCounters map[string]int    `json:"retry_counters"`
	Context       map[string]string `json:"context"`
	Logs          []string          `json:"logs,omitempty"`
	VisitLog      []VisitEntry      `json:"visit_log"`
}

// VisitEntry records a node visit.
type VisitEntry struct {
	NodeID  string      `json:"node_id"`
	Status  StageStatus `json:"status"`
	Attempt int         `json:"attempt"`
}

// Save writes the checkpoint atomically to the given path (write temp + rename).
func (cp *Checkpoint) Save(path string) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint dir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp checkpoint: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename checkpoint: %w", err)
	}

	return nil
}

// LoadCheckpoint reads a checkpoint from the given path.
func LoadCheckpoint(path string) (*Checkpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}
