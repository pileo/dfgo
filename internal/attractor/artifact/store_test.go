package artifact

import (
	"bytes"
	"os"
	"testing"
)

func TestStoreAndRetrieveSmall(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	data := []byte("hello world")
	info, err := s.Store("a1", "test.txt", data)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if info.ID != "a1" || info.Name != "test.txt" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.SizeBytes != len(data) {
		t.Fatalf("SizeBytes = %d, want %d", info.SizeBytes, len(data))
	}
	if info.IsFileBacked {
		t.Fatal("small artifact should not be file-backed")
	}

	got, err := s.Retrieve("a1")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestStoreAndRetrieveLarge(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	data := make([]byte, 150*1024) // 150KB > threshold
	for i := range data {
		data[i] = byte(i % 256)
	}

	info, err := s.Store("big", "large.bin", data)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !info.IsFileBacked {
		t.Fatal("large artifact should be file-backed")
	}

	got, err := s.Retrieve("big")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("retrieved data doesn't match stored data")
	}
}

func TestHas(t *testing.T) {
	s := NewStore(t.TempDir())

	if s.Has("x") {
		t.Fatal("Has should return false for missing artifact")
	}

	s.Store("x", "x.txt", []byte("data"))
	if !s.Has("x") {
		t.Fatal("Has should return true after store")
	}
}

func TestList(t *testing.T) {
	s := NewStore(t.TempDir())

	s.Store("a", "a.txt", []byte("a"))
	s.Store("b", "b.txt", []byte("bb"))

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Remove in-memory artifact.
	s.Store("mem", "m.txt", []byte("data"))
	s.Remove("mem")
	if s.Has("mem") {
		t.Fatal("artifact should be removed")
	}

	// Remove file-backed artifact.
	large := make([]byte, 150*1024)
	s.Store("file", "f.bin", large)
	s.Remove("file")
	if s.Has("file") {
		t.Fatal("file-backed artifact should be removed")
	}
	// File should be gone from disk.
	if _, err := os.Stat(s.filePath("file")); !os.IsNotExist(err) {
		t.Fatal("file should be deleted from disk")
	}
}

func TestClear(t *testing.T) {
	s := NewStore(t.TempDir())

	s.Store("a", "a.txt", []byte("a"))
	s.Store("b", "b.txt", []byte("b"))
	s.Clear()

	if len(s.List()) != 0 {
		t.Fatal("expected empty store after clear")
	}
}

func TestRetrieveNotFound(t *testing.T) {
	s := NewStore(t.TempDir())

	_, err := s.Retrieve("missing")
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	s := NewStore(t.TempDir())
	// Should not panic.
	s.Remove("nonexistent")
}

func TestRetrieveReturnsCopy(t *testing.T) {
	s := NewStore(t.TempDir())
	data := []byte("original")
	s.Store("id", "file.txt", data)

	got, _ := s.Retrieve("id")
	got[0] = 'X'

	got2, _ := s.Retrieve("id")
	if got2[0] != 'o' {
		t.Fatal("Retrieve should return a copy, not a reference")
	}
}
