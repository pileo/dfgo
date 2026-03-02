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
