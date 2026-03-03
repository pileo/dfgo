// Command dfgo runs Attractor pipelines from DOT files.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"dfgo/internal/agent/execenv"
	"dfgo/internal/attractor"
	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/llm"
	"dfgo/internal/llm/provider"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		logsDir     string
		autoApprove bool
		resumeRunID string
		verbose     bool
	)

	flag.StringVar(&logsDir, "logs-dir", "runs", "directory for run logs")
	flag.BoolVar(&autoApprove, "auto-approve", false, "auto-approve all human prompts")
	flag.StringVar(&resumeRunID, "resume", "", "resume a previous run by ID")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose logging")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 || args[0] != "run" {
		fmt.Fprintf(os.Stderr, "usage: dfgo run <pipeline.dot> [flags]\n")
		flag.PrintDefaults()
		return fmt.Errorf("missing required arguments")
	}

	if verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	dotFile := args[1]
	dotSource, err := os.ReadFile(dotFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", dotFile, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var iv interviewer.Interviewer
	if autoApprove {
		iv = &interviewer.AutoApprove{}
	} else {
		iv = interviewer.NewConsole()
	}

	cfg := attractor.EngineConfig{
		LogsDir:     logsDir,
		ResumeRunID: resumeRunID,
		AutoApprove: autoApprove,
		Interviewer: iv,
	}

	// Create LLM client from environment if any API keys are present.
	// This enables coding_agent pipeline nodes to call real LLMs.
	if client, err := clientFromEnv(verbose); err == nil {
		workDir, _ := filepath.Abs(".")
		env := execenv.NewLocal(workDir)
		cfg.AgentSessionFactory = handler.DefaultAgentSessionFactory(client, env)
		defer client.Close()
	}

	return attractor.RunPipeline(ctx, string(dotSource), cfg)
}

// clientFromEnv creates an LLM client from available API key environment variables.
// Returns an error if no API keys are found.
func clientFromEnv(verbose bool) (*llm.Client, error) {
	var opts []llm.ClientOption

	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		opts = append(opts, llm.WithProvider(provider.NewAnthropic()))
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		opts = append(opts, llm.WithProvider(provider.NewOpenAI()))
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		opts = append(opts, llm.WithProvider(provider.NewGemini()))
	}

	if len(opts) == 0 {
		return nil, &llm.ConfigurationError{SDKError: llm.SDKError{
			Message: "no LLM API keys found (set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY)",
		}}
	}

	// Add standard middleware.
	opts = append(opts, llm.WithRetry(llm.DefaultRetryPolicy()))
	if verbose {
		opts = append(opts, llm.WithLogging(slog.Default()))
	}

	return llm.NewClient(opts...), nil
}
