// Package llm provides a unified LLM client supporting multiple providers.
package llm

import "encoding/json"

// Role identifies the sender of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleDeveloper Role = "developer"
)

// ContentKind identifies the type of content within a message part.
type ContentKind string

const (
	ContentText       ContentKind = "text"
	ContentImage      ContentKind = "image"
	ContentToolCall   ContentKind = "tool_call"
	ContentToolResult ContentKind = "tool_result"
	ContentThinking   ContentKind = "thinking"
)

// Message represents a single message in a conversation.
type Message struct {
	Role       Role
	Content    []ContentPart
	Name       string // optional: participant name
	ToolCallID string // for tool-role messages: which call this responds to
}

// TextMessage is a convenience constructor for a simple text message.
func TextMessage(role Role, text string) Message {
	return Message{
		Role:    role,
		Content: []ContentPart{{Kind: ContentText, Text: text}},
	}
}

// Text returns the concatenated text content of the message.
func (m *Message) Text() string {
	var s string
	for _, p := range m.Content {
		if p.Kind == ContentText {
			s += p.Text
		}
	}
	return s
}

// ToolCalls returns all tool call parts in the message.
func (m *Message) ToolCalls() []ToolCallData {
	var calls []ToolCallData
	for _, p := range m.Content {
		if p.Kind == ContentToolCall && p.ToolCall != nil {
			calls = append(calls, *p.ToolCall)
		}
	}
	return calls
}

// ContentPart is one segment of a message's content.
type ContentPart struct {
	Kind       ContentKind
	Text       string
	ToolCall   *ToolCallData
	ToolResult *ToolResultData
	Thinking   *ThinkingData
	MimeType   string // for image content
	Data       []byte // for image content (base64-decoded)
}

// ToolCallData describes a tool invocation requested by the model.
type ToolCallData struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResultData describes the result of executing a tool call.
type ToolResultData struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ThinkingData holds model reasoning/thinking content.
type ThinkingData struct {
	Text      string
	Signature string // provider-specific opaque token for round-tripping
}

// ToolDef defines a tool available to the model.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
}

// ToolChoice controls which tools the model may use.
type ToolChoice struct {
	Mode     string // "auto", "none", "required", "named"
	ToolName string // only for mode="named"
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheWriteTokens int
}

// FinishReason indicates why the model stopped generating.
type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishToolUse       FinishReason = "tool_use"
	FinishLength        FinishReason = "length"
	FinishContentFilter FinishReason = "content_filter"
)
