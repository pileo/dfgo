package agent

import (
	"context"
	"testing"

	"dfgo/internal/agent/execenv"
	"dfgo/internal/agent/profile"
	"dfgo/internal/llm"
)

func TestSubagentSpawnAndWait(t *testing.T) {
	client := newTestClient(&llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, "subagent done"),
		FinishReason: llm.FinishStop,
	})

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}

	mgr := NewSubagentManager(cfg, 0)
	defer mgr.CloseAll()

	err := mgr.Spawn(context.Background(), "sub1", "do work")
	if err != nil {
		t.Fatal(err)
	}

	result, err := mgr.Wait("sub1")
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalText != "subagent done" {
		t.Errorf("final text = %q", result.FinalText)
	}
}

func TestSubagentDuplicateID(t *testing.T) {
	client := newTestClient(&llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, "done"),
		FinishReason: llm.FinishStop,
	})

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}

	mgr := NewSubagentManager(cfg, 0)
	defer mgr.CloseAll()

	mgr.Spawn(context.Background(), "sub1", "work")
	err := mgr.Spawn(context.Background(), "sub1", "more work")
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestSubagentMaxDepth(t *testing.T) {
	client := newTestClient()
	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}

	mgr := NewSubagentManager(cfg, defaultMaxDepth) // Already at max depth.
	err := mgr.Spawn(context.Background(), "deep", "work")
	if err == nil {
		t.Error("expected error for max depth")
	}
}

func TestSubagentClose(t *testing.T) {
	client := newTestClient(&llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, "done"),
		FinishReason: llm.FinishStop,
	})

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}

	mgr := NewSubagentManager(cfg, 0)
	mgr.Spawn(context.Background(), "sub1", "work")

	// Wait for it to complete before closing.
	mgr.Wait("sub1")

	err := mgr.Close("sub1")
	if err != nil {
		t.Fatal(err)
	}

	// Closing again should error.
	err = mgr.Close("sub1")
	if err == nil {
		t.Error("expected error for already-closed agent")
	}
}

func TestSubagentSendInput(t *testing.T) {
	client := newTestClient(&llm.Response{
		Message:      llm.TextMessage(llm.RoleAssistant, "done"),
		FinishReason: llm.FinishStop,
	})

	cfg := Config{
		Client:  client,
		Profile: profile.Anthropic{},
		Env:     execenv.NewLocal(t.TempDir()),
		Model:   "test-model",
	}

	mgr := NewSubagentManager(cfg, 0)
	defer mgr.CloseAll()

	mgr.Spawn(context.Background(), "sub1", "work")

	err := mgr.SendInput("sub1", "focus on tests")
	if err != nil {
		t.Fatal(err)
	}

	err = mgr.SendInput("nonexistent", "hello")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}
