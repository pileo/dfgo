// Command dfgo runs Attractor pipelines from DOT files.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"dfgo/internal/agent/execenv"
	"dfgo/internal/attractor"
	"dfgo/internal/attractor/handler"
	"dfgo/internal/attractor/interviewer"
	"dfgo/internal/llm"
	"dfgo/internal/llm/provider"
	"dfgo/internal/server"
	"dfgo/internal/server/runmgr"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "dfgo",
		Short: "Attractor pipeline orchestration engine",
	}
	root.AddCommand(runCmd())
	root.AddCommand(serveCmd())
	return root
}

func runCmd() *cobra.Command {
	var logsDir, resumeRunID, cxdbAddr string
	var autoApprove, verbose bool

	cmd := &cobra.Command{
		Use:   "run <pipeline.dot>",
		Short: "Execute an Attractor pipeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if verbose {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
			}

			dotFile := args[0]
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

			// Resolve CXDB address: flag takes priority over env var.
			if cxdbAddr == "" {
				cxdbAddr = os.Getenv("DFGO_CXDB_ADDR")
			}

			cfg := attractor.EngineConfig{
				LogsDir:     logsDir,
				ResumeRunID: resumeRunID,
				AutoApprove: autoApprove,
				Interviewer: iv,
				CXDBAddr:    cxdbAddr,
			}

			// Create LLM client from environment if any API keys are present.
			// This enables codergen and coding_agent pipeline nodes to call real LLMs.
			if client, model, err := clientFromEnv(verbose); err == nil {
				cfg.CodergenBackend = handler.NewLLMCodergenBackend(client, model)
				workDir, _ := filepath.Abs(".")
				env := execenv.NewLocal(workDir)
				cfg.AgentSessionFactory = handler.DefaultAgentSessionFactory(client, env)
				defer client.Close()
			}

			return attractor.RunPipeline(ctx, string(dotSource), cfg)
		},
	}

	cmd.Flags().StringVar(&logsDir, "logs-dir", "runs", "directory for run logs")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "auto-approve all human prompts")
	cmd.Flags().StringVar(&resumeRunID, "resume", "", "resume a previous run by ID")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "enable verbose logging")
	cmd.Flags().StringVar(&cxdbAddr, "cxdb", "", "CXDB server address (e.g., localhost:9009)")

	return cmd
}

func serveCmd() *cobra.Command {
	var addr, logsDir, cxdbAddr string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if verbose {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
			}

			// Resolve CXDB address: flag takes priority over env var.
			if cxdbAddr == "" {
				cxdbAddr = os.Getenv("DFGO_CXDB_ADDR")
			}

			baseCfg := attractor.EngineConfig{
				LogsDir:  logsDir,
				CXDBAddr: cxdbAddr,
			}

			// Create LLM client from environment if any API keys are present.
			if client, model, err := clientFromEnv(verbose); err == nil {
				baseCfg.CodergenBackend = handler.NewLLMCodergenBackend(client, model)
				workDir, _ := filepath.Abs(".")
				env := execenv.NewLocal(workDir)
				baseCfg.AgentSessionFactory = handler.DefaultAgentSessionFactory(client, env)
				defer client.Close()
			}

			srv := server.New(server.Config{
				Addr: addr,
				ManagerCfg: runmgr.ManagerConfig{
					BaseCfg: baseCfg,
				},
			})

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			// Graceful shutdown on signal.
			go func() {
				<-ctx.Done()
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer shutdownCancel()
				srv.Shutdown(shutdownCtx)
			}()

			return srv.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "listen address")
	cmd.Flags().StringVar(&logsDir, "logs-dir", "runs", "directory for run logs")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "enable verbose logging")
	cmd.Flags().StringVar(&cxdbAddr, "cxdb", "", "CXDB server address (e.g., localhost:9009)")

	return cmd
}

// clientFromEnv creates an LLM client from available API key environment variables.
// Returns the client, the default model ID for the first available provider, and an error.
func clientFromEnv(verbose bool) (*llm.Client, string, error) {
	type providerEntry struct {
		name    string
		envKey  string
		factory func() llm.ProviderAdapter
	}
	providers := []providerEntry{
		{"anthropic", "ANTHROPIC_API_KEY", func() llm.ProviderAdapter { return provider.NewAnthropic() }},
		{"openai", "OPENAI_API_KEY", func() llm.ProviderAdapter { return provider.NewOpenAI() }},
		{"gemini", "GEMINI_API_KEY", func() llm.ProviderAdapter { return provider.NewGemini() }},
	}

	var opts []llm.ClientOption
	var defaultModel string
	for _, p := range providers {
		if os.Getenv(p.envKey) == "" {
			continue
		}
		opts = append(opts, llm.WithProvider(p.factory()))
		if defaultModel == "" {
			if m, ok := llm.GetLatestModel(p.name); ok {
				defaultModel = m.ID
			}
		}
	}

	if len(opts) == 0 {
		return nil, "", &llm.ConfigurationError{SDKError: llm.SDKError{
			Message: "no LLM API keys found (set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY)",
		}}
	}

	// Add standard middleware.
	opts = append(opts, llm.WithRetry(llm.DefaultRetryPolicy()))
	if verbose {
		opts = append(opts, llm.WithLogging(slog.Default()))
	}

	return llm.NewClient(opts...), defaultModel, nil
}
