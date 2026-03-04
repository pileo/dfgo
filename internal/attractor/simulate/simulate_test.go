package simulate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dfgo/internal/attractor"
	"dfgo/internal/attractor/model"
	"dfgo/internal/attractor/runtime"
)

func TestMatch_Priority(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{NodeType: "codergen", Response: "type-match"},
			{NodeID: "review", Response: "id-match"},
			{Pattern: "test.*coverage", Response: "pattern-match"},
		},
		Fallback: "fallback-response",
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		nodeID   string
		nodeType string
		prompt   string
		want     string
	}{
		{"node ID takes priority", "review", "codergen", "test coverage", "id-match"},
		{"node type second", "other", "codergen", "anything", "type-match"},
		{"pattern third", "other", "other", "test my coverage", "pattern-match"},
		{"fallback when no match", "other", "other", "no match", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := cfg.match(tt.nodeID, tt.nodeType, tt.prompt)
			if tt.want == "" {
				if rule != nil {
					t.Errorf("expected no match, got %q", rule.Response)
				}
				return
			}
			if rule == nil {
				t.Fatal("expected a match, got nil")
			}
			if rule.Response != tt.want {
				t.Errorf("got %q, want %q", rule.Response, tt.want)
			}
		})
	}
}

func TestBackend_Generate(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{NodeID: "review", Response: "LGTM"},
			{NodeID: "broken", Status: "fail", Error: "service unavailable"},
		},
		Fallback: "default output",
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	b := NewBackend(cfg)
	ctx := context.Background()

	t.Run("matched rule returns response", func(t *testing.T) {
		resp, err := b.Generate(ctx, "anything", map[string]string{"node_id": "review"})
		if err != nil {
			t.Fatal(err)
		}
		if resp != "LGTM" {
			t.Errorf("got %q, want %q", resp, "LGTM")
		}
	})

	t.Run("fail rule returns error", func(t *testing.T) {
		_, err := b.Generate(ctx, "anything", map[string]string{"node_id": "broken"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "service unavailable" {
			t.Errorf("got %q, want %q", err.Error(), "service unavailable")
		}
	})

	t.Run("no match returns fallback", func(t *testing.T) {
		resp, err := b.Generate(ctx, "anything", map[string]string{"node_id": "unknown"})
		if err != nil {
			t.Fatal(err)
		}
		if resp != "default output" {
			t.Errorf("got %q, want %q", resp, "default output")
		}
	})
}

func TestHandler_Execute(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{NodeID: "review", Response: "LGTM", ContextUpdates: map[string]string{"reviewed": "true"}},
			{NodeID: "flaky", Status: "retry", Response: "transient failure"},
			{NodeID: "broken", Status: "fail", Error: "service unavailable"},
		},
		Fallback: "sim fallback",
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(cfg)
	ctx := context.Background()
	pctx := runtime.NewContext()
	g := &model.Graph{Name: "test"}

	t.Run("success with context updates", func(t *testing.T) {
		node := &model.Node{ID: "review", Attrs: map[string]string{"type": "codergen", "prompt": "review code"}}
		outcome, err := h.Execute(ctx, node, pctx, g, "")
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Status != runtime.StatusSuccess {
			t.Errorf("got status %q, want SUCCESS", outcome.Status)
		}
		if outcome.ContextUpdates["review.response"] != "LGTM" {
			t.Errorf("got response %q, want %q", outcome.ContextUpdates["review.response"], "LGTM")
		}
		if outcome.ContextUpdates["reviewed"] != "true" {
			t.Errorf("got reviewed %q, want %q", outcome.ContextUpdates["reviewed"], "true")
		}
	})

	t.Run("retry outcome", func(t *testing.T) {
		node := &model.Node{ID: "flaky", Attrs: map[string]string{"type": "codergen", "prompt": "do something"}}
		outcome, err := h.Execute(ctx, node, pctx, g, "")
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Status != runtime.StatusRetry {
			t.Errorf("got status %q, want RETRY", outcome.Status)
		}
	})

	t.Run("fail outcome", func(t *testing.T) {
		node := &model.Node{ID: "broken", Attrs: map[string]string{"type": "codergen", "prompt": "do something"}}
		outcome, err := h.Execute(ctx, node, pctx, g, "")
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Status != runtime.StatusFail {
			t.Errorf("got status %q, want FAIL", outcome.Status)
		}
		if outcome.FailureReason != "service unavailable" {
			t.Errorf("got reason %q, want %q", outcome.FailureReason, "service unavailable")
		}
	})

	t.Run("fallback when no match", func(t *testing.T) {
		node := &model.Node{ID: "unknown", Attrs: map[string]string{"type": "codergen", "prompt": "do something"}}
		outcome, err := h.Execute(ctx, node, pctx, g, "")
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Status != runtime.StatusSuccess {
			t.Errorf("got status %q, want SUCCESS", outcome.Status)
		}
		if outcome.ContextUpdates["unknown.response"] != "sim fallback" {
			t.Errorf("got response %q, want %q", outcome.ContextUpdates["unknown.response"], "sim fallback")
		}
	})
}

func TestHandler_Delay(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{NodeID: "slow", Response: "done", Delay: "50ms"},
		},
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(cfg)
	node := &model.Node{ID: "slow", Attrs: map[string]string{"type": "codergen", "prompt": "go"}}
	pctx := runtime.NewContext()
	g := &model.Graph{Name: "test"}

	start := time.Now()
	outcome, err := h.Execute(context.Background(), node, pctx, g, "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != runtime.StatusSuccess {
		t.Errorf("got status %q, want SUCCESS", outcome.Status)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("delay too short: %v", elapsed)
	}
}

func TestHandler_DelayCancellation(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{NodeID: "slow", Response: "done", Delay: "5s"},
		},
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(cfg)
	node := &model.Node{ID: "slow", Attrs: map[string]string{"type": "codergen", "prompt": "go"}}
	pctx := runtime.NewContext()
	g := &model.Graph{Name: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := h.Execute(ctx, node, pctx, g, "")
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sim.json")

	t.Run("valid config", func(t *testing.T) {
		data := `{
			"rules": [
				{"node_id": "A", "response": "hello"},
				{"pattern": "test.*", "response": "matched"}
			],
			"fallback": "default"
		}`
		os.WriteFile(cfgPath, []byte(data), 0o644)

		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Rules) != 2 {
			t.Errorf("got %d rules, want 2", len(cfg.Rules))
		}
		if cfg.Fallback != "default" {
			t.Errorf("got fallback %q, want %q", cfg.Fallback, "default")
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		data := `{"rules": [{"pattern": "[invalid", "response": "x"}]}`
		os.WriteFile(cfgPath, []byte(data), 0o644)

		_, err := LoadConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for invalid regex")
		}
	})

	t.Run("invalid delay", func(t *testing.T) {
		data := `{"rules": [{"node_id": "x", "response": "x", "delay": "notaduration"}]}`
		os.WriteFile(cfgPath, []byte(data), 0o644)

		_, err := LoadConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for invalid delay")
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		data := `{"rules": [{"node_id": "x", "response": "x", "status": "unknown"}]}`
		os.WriteFile(cfgPath, []byte(data), 0o644)

		_, err := LoadConfig(cfgPath)
		if err == nil {
			t.Fatal("expected error for invalid status")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

func TestBuildRegistry(t *testing.T) {
	cfg := &Config{
		Rules:    []Rule{{NodeID: "A", Response: "simulated"}},
		Fallback: "fallback",
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	reg := BuildRegistry(cfg)

	// codergen node should resolve to the simulation handler.
	codergenNode := &model.Node{ID: "A", Attrs: map[string]string{"type": "codergen"}}
	h, err := reg.Lookup(codergenNode)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*Handler); !ok {
		t.Errorf("expected *Handler, got %T", h)
	}

	// coding_agent node should also resolve to the simulation handler.
	agentNode := &model.Node{ID: "B", Attrs: map[string]string{"type": "coding_agent"}}
	h, err = reg.Lookup(agentNode)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.(*Handler); !ok {
		t.Errorf("expected *Handler, got %T", h)
	}

	// start node should still work (shape-based).
	startNode := &model.Node{ID: "start", Attrs: map[string]string{"shape": "Mdiamond"}}
	_, err = reg.Lookup(startNode)
	if err != nil {
		t.Errorf("expected start handler, got error: %v", err)
	}
}

func TestIntegration_SimulatedPipeline(t *testing.T) {
	cfg := &Config{
		Rules: []Rule{
			{NodeID: "A", Response: "step A done", ContextUpdates: map[string]string{"a_done": "true"}},
			{NodeID: "B", Response: "step B done"},
		},
		Fallback: "unexpected node",
	}
	if err := cfg.compile(); err != nil {
		t.Fatal(err)
	}

	dotSource := `digraph test {
		start [shape=Mdiamond]
		A [shape=box, type="codergen", prompt="Do step A"]
		B [shape=box, type="codergen", prompt="Do step B"]
		exit [shape=Msquare]

		start -> A -> B -> exit
	}`

	reg := BuildRegistry(cfg)

	err := attractor.RunPipeline(context.Background(), dotSource, attractor.EngineConfig{
		Registry:    reg,
		AutoApprove: true,
		LogsDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
}
