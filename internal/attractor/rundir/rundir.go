// Package rundir manages run directory creation and manifest writing.
package rundir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest describes a pipeline run.
type Manifest struct {
	RunID     string    `json:"run_id"`
	Pipeline  string    `json:"pipeline"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status,omitempty"`
	NodeCount int       `json:"node_count,omitempty"`
}

// RunDir manages the directory structure for a pipeline run.
type RunDir struct {
	Root string
}

// Create creates the run directory hierarchy:
//
//	{logsRoot}/{runID}/
//	{logsRoot}/{runID}/artifacts/
func Create(logsRoot, runID string) (*RunDir, error) {
	root := filepath.Join(logsRoot, runID)
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	return &RunDir{Root: root}, nil
}

// NodeDir creates and returns the path for a node's log directory.
func (rd *RunDir) NodeDir(nodeID string) (string, error) {
	dir := filepath.Join(rd.Root, nodeID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create node dir: %w", err)
	}
	return dir, nil
}

// ArtifactsDir returns the path to the artifacts directory.
func (rd *RunDir) ArtifactsDir() string {
	return filepath.Join(rd.Root, "artifacts")
}

// CheckpointPath returns the path for the checkpoint file.
func (rd *RunDir) CheckpointPath() string {
	return filepath.Join(rd.Root, "checkpoint.json")
}

// WriteManifest writes the manifest.json to the run directory.
func (rd *RunDir) WriteManifest(m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(rd.Root, "manifest.json"), data, 0o644)
}
