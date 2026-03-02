package rundir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	rd, err := Create(dir, "run-001")
	if err != nil {
		t.Fatal(err)
	}
	// Check root exists
	info, err := os.Stat(rd.Root)
	if err != nil || !info.IsDir() {
		t.Fatal("expected run dir to exist")
	}
	// Check artifacts dir
	info, err = os.Stat(rd.ArtifactsDir())
	if err != nil || !info.IsDir() {
		t.Fatal("expected artifacts dir")
	}
}

func TestNodeDir(t *testing.T) {
	dir := t.TempDir()
	rd, err := Create(dir, "run-002")
	if err != nil {
		t.Fatal(err)
	}
	nodeDir, err := rd.NodeDir("step_A")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(nodeDir)
	if err != nil || !info.IsDir() {
		t.Fatal("expected node dir")
	}
	if filepath.Base(nodeDir) != "step_A" {
		t.Fatalf("unexpected node dir name: %s", nodeDir)
	}
}

func TestWriteManifest(t *testing.T) {
	dir := t.TempDir()
	rd, err := Create(dir, "run-003")
	if err != nil {
		t.Fatal(err)
	}

	m := Manifest{
		RunID:     "run-003",
		Pipeline:  "test.dot",
		StartedAt: time.Now().Truncate(time.Second),
		Status:    "running",
		NodeCount: 5,
	}
	if err := rd.WriteManifest(m); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(rd.Root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var loaded Manifest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != "run-003" || loaded.Pipeline != "test.dot" {
		t.Fatal("manifest content mismatch")
	}
}

func TestCheckpointPath(t *testing.T) {
	rd := &RunDir{Root: "/tmp/runs/run-001"}
	if rd.CheckpointPath() != "/tmp/runs/run-001/checkpoint.json" {
		t.Fatal("unexpected checkpoint path")
	}
}
