package llm

import "encoding/json"

// Request describes a completion request to an LLM provider.
type Request struct {
	Model           string
	Messages        []Message
	Provider        string         // optional: route to specific provider
	Tools           []ToolDef
	ToolChoice      *ToolChoice
	Temperature     *float64
	MaxTokens       *int
	TopP            *float64
	ReasoningEffort string         // "low", "medium", "high"
	Stop            []string       // stop sequences
	ProviderOptions map[string]any // provider-specific escape hatch
}

// Response holds the result of a completion request.
type Response struct {
	ID           string
	Model        string
	Provider     string
	Message      Message
	FinishReason FinishReason
	Usage        Usage
	Raw          json.RawMessage // raw provider response for debugging
}

// Text returns the concatenated text from the response message.
func (r *Response) Text() string {
	return r.Message.Text()
}

// ToolCalls returns all tool calls from the response message.
func (r *Response) ToolCalls() []ToolCallData {
	return r.Message.ToolCalls()
}
