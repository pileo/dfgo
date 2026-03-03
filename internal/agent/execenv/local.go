package execenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	maxTimeout     = 10 * time.Minute
	killGrace      = 2 * time.Second
)

// filteredEnvVars are environment variables stripped from child processes.
var filteredEnvVars = map[string]bool{
	"ANTHROPIC_API_KEY": true,
	"OPENAI_API_KEY":    true,
	"GEMINI_API_KEY":    true,
}

// Local implements Environment using the local filesystem and os/exec.
type Local struct {
	workDir string
}

// NewLocal creates a Local environment rooted at the given directory.
func NewLocal(workDir string) *Local {
	return &Local{workDir: workDir}
}

func (l *Local) WorkingDir() string { return l.workDir }

func (l *Local) ReadFile(_ context.Context, path string) ([]byte, error) {
	resolved := l.resolve(path)
	return os.ReadFile(resolved)
}

func (l *Local) WriteFile(_ context.Context, path string, data []byte, perm int) error {
	resolved := l.resolve(path)
	if perm == 0 {
		perm = 0644
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(resolved, data, os.FileMode(perm))
}

func (l *Local) Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error) {
	timeout := defaultTimeout
	if opts.Timeout > 0 {
		timeout = time.Duration(opts.Timeout) * time.Second
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dir := l.workDir
	if opts.Dir != "" {
		dir = l.resolve(opts.Dir)
	}

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = filteredEnv()
	// Set process group so we can kill the entire tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		return ExecResult{}, fmt.Errorf("failed to start command: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err = <-done:
		// Command completed.
	case <-cmdCtx.Done():
		// Timeout: SIGTERM → wait grace → SIGKILL.
		_ = killProcessGroup(cmd.Process.Pid)
		select {
		case <-done:
		case <-time.After(killGrace):
			_ = forceKillProcessGroup(cmd.Process.Pid)
			<-done
		}
		return ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: -1,
		}, fmt.Errorf("command timed out after %s", timeout)
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: -1,
			}, err
		}
	}

	return ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

func (l *Local) Glob(_ context.Context, pattern string) ([]string, error) {
	resolved := l.resolve(pattern)
	matches, err := filepath.Glob(resolved)
	if err != nil {
		return nil, err
	}
	// Return paths relative to working directory.
	rel := make([]string, 0, len(matches))
	for _, m := range matches {
		r, err := filepath.Rel(l.workDir, m)
		if err != nil {
			rel = append(rel, m)
		} else {
			rel = append(rel, r)
		}
	}
	return rel, nil
}

func (l *Local) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.workDir, path)
}

func filteredEnv() []string {
	env := os.Environ()
	result := make([]string, 0, len(env))
	for _, e := range env {
		key := e[:strings.IndexByte(e, '=')]
		if !filteredEnvVars[key] {
			result = append(result, e)
		}
	}
	return result
}

func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func forceKillProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
