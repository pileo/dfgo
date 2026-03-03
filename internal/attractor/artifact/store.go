// Package artifact provides a simple artifact store for pipeline runs.
// Small artifacts (< 100KB) are held in memory; larger ones are file-backed.
package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fileBackingThreshold is the size above which artifacts are stored on disk.
const fileBackingThreshold = 100 * 1024 // 100KB

// ArtifactInfo describes a stored artifact.
type ArtifactInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	SizeBytes   int       `json:"size_bytes"`
	StoredAt    time.Time `json:"stored_at"`
	IsFileBacked bool    `json:"is_file_backed"`
}

// Store manages artifacts for a pipeline run.
type Store struct {
	mu      sync.RWMutex
	baseDir string
	memory  map[string][]byte
	info    map[string]ArtifactInfo
}

// NewStore creates a new artifact store backed by the given directory.
func NewStore(baseDir string) *Store {
	return &Store{
		baseDir: baseDir,
		memory:  make(map[string][]byte),
		info:    make(map[string]ArtifactInfo),
	}
}

// Store saves an artifact. Returns metadata about the stored artifact.
func (s *Store) Store(id, name string, data []byte) (ArtifactInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := ArtifactInfo{
		ID:        id,
		Name:      name,
		SizeBytes: len(data),
		StoredAt:  time.Now(),
	}

	if len(data) >= fileBackingThreshold {
		info.IsFileBacked = true
		if err := s.writeFile(id, data); err != nil {
			return ArtifactInfo{}, err
		}
	} else {
		cp := make([]byte, len(data))
		copy(cp, data)
		s.memory[id] = cp
	}

	s.info[id] = info
	return info, nil
}

// Retrieve returns the data for the given artifact ID.
func (s *Store) Retrieve(id string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, ok := s.info[id]
	if !ok {
		return nil, fmt.Errorf("artifact %q not found", id)
	}

	if info.IsFileBacked {
		return os.ReadFile(s.filePath(id))
	}

	data, ok := s.memory[id]
	if !ok {
		return nil, fmt.Errorf("artifact %q not found in memory", id)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

// Has returns true if the artifact exists in the store.
func (s *Store) Has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.info[id]
	return ok
}

// List returns metadata for all stored artifacts.
func (s *Store) List() []ArtifactInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ArtifactInfo, 0, len(s.info))
	for _, info := range s.info {
		out = append(out, info)
	}
	return out
}

// Remove deletes an artifact from the store.
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, ok := s.info[id]
	if !ok {
		return
	}
	if info.IsFileBacked {
		os.Remove(s.filePath(id))
	}
	delete(s.memory, id)
	delete(s.info, id)
}

// Clear removes all artifacts from the store.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, info := range s.info {
		if info.IsFileBacked {
			os.Remove(s.filePath(id))
		}
		delete(s.memory, id)
		delete(s.info, id)
	}
}

func (s *Store) filePath(id string) string {
	return filepath.Join(s.baseDir, id+".dat")
}

func (s *Store) writeFile(id string, data []byte) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	return os.WriteFile(s.filePath(id), data, 0o644)
}
