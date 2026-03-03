package prompt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxDiscoveredBytes = 32 * 1024

// providerDocFiles maps provider names to their project instruction files.
var providerDocFiles = map[string][]string{
	"anthropic": {"CLAUDE.md"},
	"openai":    {".codex/instructions.md"},
	"gemini":    {"GEMINI.md"},
}

// DiscoverProjectDocs auto-discovers project instruction files by walking
// from the git root to workDir. It looks for AGENTS.md (always) and
// provider-specific files based on providerName.
// Returns concatenated contents (root first, deeper files appended),
// truncated at 32KB.
func DiscoverProjectDocs(workDir string, providerName string) string {
	root := gitRoot(workDir)
	if root == "" {
		root = workDir
	}

	// Build list of filenames to look for.
	filenames := []string{"AGENTS.md"}
	if extras, ok := providerDocFiles[providerName]; ok {
		filenames = append(filenames, extras...)
	}

	// Walk directories from root to workDir.
	dirs := pathChain(root, workDir)

	var parts []string
	totalLen := 0
	for _, dir := range dirs {
		for _, name := range filenames {
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == "" {
				continue
			}
			if totalLen+len(content) > maxDiscoveredBytes {
				remaining := maxDiscoveredBytes - totalLen
				if remaining > 0 {
					parts = append(parts, content[:remaining]+"\n... [truncated]")
				}
				return strings.Join(parts, "\n\n")
			}
			parts = append(parts, content)
			totalLen += len(content)
		}
	}

	return strings.Join(parts, "\n\n")
}

// gitRoot returns the git repository root for the given directory,
// or empty string if not in a git repo.
func gitRoot(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// pathChain returns the list of directories from root to target (inclusive).
// If target is not under root, returns just [target].
func pathChain(root, target string) []string {
	root = filepath.Clean(root)
	target = filepath.Clean(target)

	if root == target {
		return []string{root}
	}

	rel, err := filepath.Rel(root, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return []string{target}
	}

	dirs := []string{root}
	parts := strings.Split(rel, string(filepath.Separator))
	cur := root
	for _, p := range parts {
		cur = filepath.Join(cur, p)
		dirs = append(dirs, cur)
	}
	return dirs
}
