package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathChainSameDir(t *testing.T) {
	dirs := pathChain("/a/b", "/a/b")
	if len(dirs) != 1 || dirs[0] != "/a/b" {
		t.Errorf("pathChain same dir = %v, want [/a/b]", dirs)
	}
}

func TestPathChainNested(t *testing.T) {
	dirs := pathChain("/a/b", "/a/b/c/d")
	want := []string{"/a/b", "/a/b/c", "/a/b/c/d"}
	if len(dirs) != len(want) {
		t.Fatalf("pathChain = %v, want %v", dirs, want)
	}
	for i, d := range dirs {
		if d != want[i] {
			t.Errorf("pathChain[%d] = %q, want %q", i, d, want[i])
		}
	}
}

func TestPathChainNotUnder(t *testing.T) {
	dirs := pathChain("/a/b", "/c/d")
	if len(dirs) != 1 || dirs[0] != "/c/d" {
		t.Errorf("pathChain not-under = %v, want [/c/d]", dirs)
	}
}

func TestDiscoverProjectDocsNoFiles(t *testing.T) {
	dir := t.TempDir()
	result := DiscoverProjectDocs(dir, "anthropic")
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestDiscoverProjectDocsWithFile(t *testing.T) {
	dir := t.TempDir()
	content := "# Agent Instructions\nUse tabs."
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverProjectDocs(dir, "anthropic")
	if !strings.Contains(result, "Agent Instructions") {
		t.Errorf("expected AGENTS.md content, got %q", result)
	}
	if !strings.Contains(result, "Use tabs.") {
		t.Errorf("expected full content, got %q", result)
	}
}

func TestDiscoverProjectDocsProviderFile(t *testing.T) {
	dir := t.TempDir()
	content := "# Claude Instructions\nBe helpful."
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverProjectDocs(dir, "anthropic")
	if !strings.Contains(result, "Claude Instructions") {
		t.Errorf("expected CLAUDE.md content for anthropic provider, got %q", result)
	}
}

func TestDiscoverProjectDocsWrongProvider(t *testing.T) {
	dir := t.TempDir()
	// CLAUDE.md should not be picked up for "openai" provider.
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude stuff"), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverProjectDocs(dir, "openai")
	if strings.Contains(result, "claude stuff") {
		t.Error("CLAUDE.md should not be discovered for openai provider")
	}
}

func TestDiscoverProjectDocsTruncation(t *testing.T) {
	dir := t.TempDir()
	// Create an oversized file (bigger than 32KB budget).
	bigContent := strings.Repeat("x", 40000)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(bigContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverProjectDocs(dir, "anthropic")
	if !strings.Contains(result, "... [truncated]") {
		t.Error("expected truncation marker for oversized file")
	}
	if len(result) > maxDiscoveredBytes+100 {
		t.Errorf("result too large: %d bytes", len(result))
	}
}

func TestDiscoverProjectDocsMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude content"), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverProjectDocs(dir, "anthropic")
	if !strings.Contains(result, "agents content") {
		t.Error("missing AGENTS.md content")
	}
	if !strings.Contains(result, "claude content") {
		t.Error("missing CLAUDE.md content")
	}
}

func TestDiscoverProjectDocsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("   "), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverProjectDocs(dir, "anthropic")
	if result != "" {
		t.Errorf("expected empty result for whitespace-only file, got %q", result)
	}
}
