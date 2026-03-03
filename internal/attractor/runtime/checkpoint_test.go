package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")

	cp := &Checkpoint{
		RunID:       "run-123",
		CurrentNode: "A",
		Completed:   []string{"start"},
		RetryCounters: map[string]int{
			"A": 1,
		},
		Context: map[string]string{
			"key": "value",
		},
		VisitLog: []VisitEntry{
			{NodeID: "start", Status: StatusSuccess, Attempt: 1},
		},
	}

	if err := cp.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.RunID != "run-123" {
		t.Fatalf("expected run-123, got %s", loaded.RunID)
	}
	if loaded.CurrentNode != "A" {
		t.Fatal("expected current node A")
	}
	if len(loaded.Completed) != 1 || loaded.Completed[0] != "start" {
		t.Fatal("expected completed=[start]")
	}
	if loaded.RetryCounters["A"] != 1 {
		t.Fatal("expected retry counter A=1")
	}
	if loaded.Context["key"] != "value" {
		t.Fatal("expected context key=value")
	}
	if len(loaded.VisitLog) != 1 || loaded.VisitLog[0].NodeID != "start" {
		t.Fatal("expected visit log")
	}
}

func TestCheckpointAtomicity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")

	cp := &Checkpoint{RunID: "run-1"}
	if err := cp.Save(path); err != nil {
		t.Fatal(err)
	}

	// Verify no temp file remains
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp file should not exist after successful save")
	}
}

func TestLoadCheckpointNotFound(t *testing.T) {
	_, err := LoadCheckpoint("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestCheckpointWithLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")

	cp := &Checkpoint{
		RunID:         "run-logs",
		CurrentNode:   "B",
		Completed:     []string{"A"},
		RetryCounters: map[string]int{},
		Context:       map[string]string{},
		Logs:          []string{"entry1", "entry2"},
		VisitLog: []VisitEntry{
			{NodeID: "A", Status: StatusSuccess, Attempt: 1},
		},
	}

	if err := cp.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(loaded.Logs))
	}
	if loaded.Logs[0] != "entry1" || loaded.Logs[1] != "entry2" {
		t.Fatalf("unexpected logs: %v", loaded.Logs)
	}
}

func TestCheckpointEmptyLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")

	cp := &Checkpoint{
		RunID:         "run-no-logs",
		CurrentNode:   "A",
		Completed:     []string{},
		RetryCounters: map[string]int{},
		Context:       map[string]string{},
		VisitLog:      []VisitEntry{},
	}

	if err := cp.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Logs != nil && len(loaded.Logs) != 0 {
		t.Fatalf("expected nil or empty logs, got %v", loaded.Logs)
	}
}
