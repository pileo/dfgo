// Package message defines the agent-level message types.
// These are simpler than the LLM SDK's multimodal Message type;
// the Session translates between them.
package message

import "encoding/json"

// Role identifies the sender of a message in the agent loop.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSteering  Role = "steering"
)

// Message represents a single message in the agent conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolResult *ToolResult
	IsSteering bool // true for system-injected steering messages
}

// ToolCall describes a tool invocation requested by the model.
type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

// ToolResult describes the result of executing a tool call.
type ToolResult struct {
	ToolCallID string
	Content    string
	FullOutput string // pre-truncation output for logging
	IsError    bool
}

// UserMessage creates a simple user message.
func UserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

// AssistantMessage creates a simple assistant message.
func AssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

// SteeringMessage creates a system-injected steering message.
func SteeringMessage(content string) Message {
	return Message{Role: RoleSteering, Content: content, IsSteering: true}
}

// ToolResultMessage creates a tool result message.
func ToolResultMessage(callID, content string, isError bool) Message {
	return Message{
		Role: RoleTool,
		ToolResult: &ToolResult{
			ToolCallID: callID,
			Content:    content,
			IsError:    isError,
		},
	}
}
