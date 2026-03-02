// Command dfgo runs Attractor pipelines from DOT files.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"dfgo/internal/attractor"
	"dfgo/internal/attractor/interviewer"
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

	return attractor.RunPipeline(ctx, string(dotSource), cfg)
}
