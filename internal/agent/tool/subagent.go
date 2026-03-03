package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"dfgo/internal/agent/execenv"
)

// SubagentSpawner is the interface tools need to manage subagents.
// This avoids a circular dependency on the agent package.
type SubagentSpawner interface {
	Spawn(ctx context.Context, id string, input string) error
	SendInput(id string, input string) error
	Wait(id string) (SubagentResult, error)
	Close(id string) error
}

// SubagentResult is a minimal result returned by Wait.
type SubagentResult struct {
	FinalText string
	Rounds    int
	Aborted   bool
	Error     error
}

// --- spawn_agent ---

type spawnAgentTool struct {
	mgr SubagentSpawner
}

func NewSpawnAgent(mgr SubagentSpawner) Tool { return &spawnAgentTool{mgr: mgr} }

func (t *spawnAgentTool) Name() string        { return "spawn_agent" }
func (t *spawnAgentTool) Description() string { return "Spawn a child agent to work on a subtask" }
func (t *spawnAgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {"type": "string", "description": "The task for the child agent to perform"},
			"agent_id": {"type": "string", "description": "Unique identifier for this agent"}
		},
		"required": ["task", "agent_id"]
	}`)
}

func (t *spawnAgentTool) Execute(ctx context.Context, _ execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		Task    string `json:"task"`
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.Task == "" {
		return ErrorResult("task is required"), nil
	}
	if params.AgentID == "" {
		return ErrorResult("agent_id is required"), nil
	}
	if err := t.mgr.Spawn(ctx, params.AgentID, params.Task); err != nil {
		return ErrorResult(fmt.Sprintf("spawn failed: %v", err)), nil
	}
	return SuccessResult("spawn_agent", fmt.Sprintf("Agent %q spawned successfully", params.AgentID)), nil
}

// --- send_input ---

type sendInputTool struct {
	mgr SubagentSpawner
}

func NewSendInput(mgr SubagentSpawner) Tool { return &sendInputTool{mgr: mgr} }

func (t *sendInputTool) Name() string        { return "send_input" }
func (t *sendInputTool) Description() string { return "Send additional input to a running child agent" }
func (t *sendInputTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_id": {"type": "string", "description": "ID of the target agent"},
			"message": {"type": "string", "description": "Message to send"}
		},
		"required": ["agent_id", "message"]
	}`)
}

func (t *sendInputTool) Execute(_ context.Context, _ execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		AgentID string `json:"agent_id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.AgentID == "" || params.Message == "" {
		return ErrorResult("agent_id and message are required"), nil
	}
	if err := t.mgr.SendInput(params.AgentID, params.Message); err != nil {
		return ErrorResult(fmt.Sprintf("send_input failed: %v", err)), nil
	}
	return SuccessResult("send_input", "Input sent successfully"), nil
}

// --- wait ---

type waitTool struct {
	mgr SubagentSpawner
}

func NewWaitAgent(mgr SubagentSpawner) Tool { return &waitTool{mgr: mgr} }

func (t *waitTool) Name() string        { return "wait" }
func (t *waitTool) Description() string { return "Wait for a child agent to complete and return its result" }
func (t *waitTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_id": {"type": "string", "description": "ID of the agent to wait for"}
		},
		"required": ["agent_id"]
	}`)
}

func (t *waitTool) Execute(_ context.Context, _ execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.AgentID == "" {
		return ErrorResult("agent_id is required"), nil
	}
	result, err := t.mgr.Wait(params.AgentID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("wait failed: %v", err)), nil
	}

	status := "completed"
	if result.Aborted {
		status = "aborted"
	}
	if result.Error != nil {
		status = fmt.Sprintf("error: %v", result.Error)
	}

	output := fmt.Sprintf("Agent %q finished (%s, %d rounds).\n\n%s",
		params.AgentID, status, result.Rounds, result.FinalText)
	return SuccessResult("wait", output), nil
}

// --- close_agent ---

type closeAgentTool struct {
	mgr SubagentSpawner
}

func NewCloseAgent(mgr SubagentSpawner) Tool { return &closeAgentTool{mgr: mgr} }

func (t *closeAgentTool) Name() string        { return "close_agent" }
func (t *closeAgentTool) Description() string { return "Cancel and clean up a child agent" }
func (t *closeAgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_id": {"type": "string", "description": "ID of the agent to close"}
		},
		"required": ["agent_id"]
	}`)
}

func (t *closeAgentTool) Execute(_ context.Context, _ execenv.Environment, args json.RawMessage) (Result, error) {
	var params struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
	}
	if params.AgentID == "" {
		return ErrorResult("agent_id is required"), nil
	}
	if err := t.mgr.Close(params.AgentID); err != nil {
		return ErrorResult(fmt.Sprintf("close failed: %v", err)), nil
	}
	return SuccessResult("close_agent", fmt.Sprintf("Agent %q closed", params.AgentID)), nil
}
