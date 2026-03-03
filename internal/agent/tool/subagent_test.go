package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// mockSubagentSpawner implements SubagentSpawner for testing.
type mockSubagentSpawner struct {
	spawnCalled bool
	spawnID     string
	spawnInput  string
	spawnErr    error

	sendCalled  bool
	sendID      string
	sendMessage string
	sendErr     error

	waitCalled bool
	waitID     string
	waitResult SubagentResult
	waitErr    error

	closeCalled bool
	closeID     string
	closeErr    error
}

func (m *mockSubagentSpawner) Spawn(_ context.Context, id, input string) error {
	m.spawnCalled = true
	m.spawnID = id
	m.spawnInput = input
	return m.spawnErr
}

func (m *mockSubagentSpawner) SendInput(id, input string) error {
	m.sendCalled = true
	m.sendID = id
	m.sendMessage = input
	return m.sendErr
}

func (m *mockSubagentSpawner) Wait(id string) (SubagentResult, error) {
	m.waitCalled = true
	m.waitID = id
	return m.waitResult, m.waitErr
}

func (m *mockSubagentSpawner) Close(id string) error {
	m.closeCalled = true
	m.closeID = id
	return m.closeErr
}

func TestSpawnAgent(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewSpawnAgent(mgr)
	ctx := context.Background()

	result, err := tool.Execute(ctx, nil, json.RawMessage(`{"task":"write tests","agent_id":"agent-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !mgr.spawnCalled {
		t.Error("Spawn was not called")
	}
	if mgr.spawnID != "agent-1" {
		t.Errorf("spawn id = %q, want %q", mgr.spawnID, "agent-1")
	}
	if mgr.spawnInput != "write tests" {
		t.Errorf("spawn input = %q, want %q", mgr.spawnInput, "write tests")
	}
	if !strings.Contains(result.Content, "agent-1") {
		t.Errorf("result should mention agent id, got %q", result.Content)
	}
}

func TestSpawnAgentMissingTask(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewSpawnAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"task":"","agent_id":"agent-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for empty task")
	}
	if mgr.spawnCalled {
		t.Error("Spawn should not be called for empty task")
	}
}

func TestSpawnAgentMissingID(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewSpawnAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"task":"do stuff","agent_id":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for empty agent_id")
	}
	if mgr.spawnCalled {
		t.Error("Spawn should not be called for empty agent_id")
	}
}

func TestSpawnAgentInvalidJSON(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewSpawnAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestSpawnAgentSpawnError(t *testing.T) {
	mgr := &mockSubagentSpawner{spawnErr: fmt.Errorf("capacity exceeded")}
	tool := NewSpawnAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"task":"work","agent_id":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for spawn failure")
	}
	if !strings.Contains(result.Content, "capacity exceeded") {
		t.Errorf("error should mention cause, got %q", result.Content)
	}
}

func TestSendInput(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewSendInput(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"agent-1","message":"more context"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !mgr.sendCalled {
		t.Error("SendInput was not called")
	}
	if mgr.sendID != "agent-1" {
		t.Errorf("send id = %q", mgr.sendID)
	}
	if mgr.sendMessage != "more context" {
		t.Errorf("send message = %q", mgr.sendMessage)
	}
}

func TestSendInputMissingArgs(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewSendInput(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"","message":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for empty agent_id and message")
	}
}

func TestSendInputError(t *testing.T) {
	mgr := &mockSubagentSpawner{sendErr: fmt.Errorf("agent not found")}
	tool := NewSendInput(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"x","message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
	if !strings.Contains(result.Content, "agent not found") {
		t.Errorf("error should mention cause, got %q", result.Content)
	}
}

func TestWaitAgent(t *testing.T) {
	mgr := &mockSubagentSpawner{
		waitResult: SubagentResult{
			FinalText: "task completed successfully",
			Rounds:    5,
			Aborted:   false,
		},
	}
	tool := NewWaitAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"agent-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !mgr.waitCalled {
		t.Error("Wait was not called")
	}
	if mgr.waitID != "agent-1" {
		t.Errorf("wait id = %q", mgr.waitID)
	}
	if !strings.Contains(result.Content, "completed") {
		t.Error("result should indicate completed status")
	}
	if !strings.Contains(result.Content, "5 rounds") {
		t.Error("result should mention rounds")
	}
	if !strings.Contains(result.Content, "task completed successfully") {
		t.Error("result should include final text")
	}
}

func TestWaitAgentAborted(t *testing.T) {
	mgr := &mockSubagentSpawner{
		waitResult: SubagentResult{
			FinalText: "partial work",
			Rounds:    3,
			Aborted:   true,
		},
	}
	tool := NewWaitAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"agent-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "aborted") {
		t.Error("result should indicate aborted status")
	}
}

func TestWaitAgentWithError(t *testing.T) {
	mgr := &mockSubagentSpawner{
		waitResult: SubagentResult{
			FinalText: "",
			Rounds:    1,
			Error:     fmt.Errorf("runtime panic"),
		},
	}
	tool := NewWaitAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"agent-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "runtime panic") {
		t.Errorf("result should contain error detail, got %q", result.Content)
	}
}

func TestWaitAgentMissingID(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewWaitAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for empty agent_id")
	}
}

func TestWaitAgentWaitError(t *testing.T) {
	mgr := &mockSubagentSpawner{waitErr: fmt.Errorf("no such agent")}
	tool := NewWaitAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
	if !strings.Contains(result.Content, "no such agent") {
		t.Errorf("error should mention cause, got %q", result.Content)
	}
}

func TestCloseAgent(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewCloseAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"agent-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !mgr.closeCalled {
		t.Error("Close was not called")
	}
	if mgr.closeID != "agent-1" {
		t.Errorf("close id = %q", mgr.closeID)
	}
	if !strings.Contains(result.Content, "agent-1") {
		t.Errorf("result should mention agent id, got %q", result.Content)
	}
}

func TestCloseAgentMissingID(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tool := NewCloseAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for empty agent_id")
	}
}

func TestCloseAgentError(t *testing.T) {
	mgr := &mockSubagentSpawner{closeErr: fmt.Errorf("already closed")}
	tool := NewCloseAgent(mgr)

	result, err := tool.Execute(context.Background(), nil, json.RawMessage(`{"agent_id":"a1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
	if !strings.Contains(result.Content, "already closed") {
		t.Errorf("error should mention cause, got %q", result.Content)
	}
}

func TestToolNames(t *testing.T) {
	mgr := &mockSubagentSpawner{}
	tests := []struct {
		tool Tool
		name string
	}{
		{NewSpawnAgent(mgr), "spawn_agent"},
		{NewSendInput(mgr), "send_input"},
		{NewWaitAgent(mgr), "wait"},
		{NewCloseAgent(mgr), "close_agent"},
	}
	for _, tt := range tests {
		if got := tt.tool.Name(); got != tt.name {
			t.Errorf("tool name = %q, want %q", got, tt.name)
		}
		if tt.tool.Description() == "" {
			t.Errorf("tool %q has empty description", tt.name)
		}
		if len(tt.tool.Parameters()) == 0 {
			t.Errorf("tool %q has empty parameters", tt.name)
		}
	}
}
