// Package provider implements LLM provider adapters for the unified client.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"dfgo/internal/llm"
)

const (
	anthropicDefaultURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion     = "2023-06-01"
	anthropicDefaultMaxToks = 4096
)

// Anthropic implements the Anthropic Messages API.
type Anthropic struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewAnthropic creates an Anthropic provider, reading API key from env if not set.
func NewAnthropic(opts ...func(*Anthropic)) *Anthropic {
	a := &Anthropic{
		BaseURL: anthropicDefaultURL,
		Client:  http.DefaultClient,
	}
	for _, o := range opts {
		o(a)
	}
	if a.APIKey == "" {
		a.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return a
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	body, err := a.buildRequest(req)
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to build anthropic request", Cause: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to create http request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	httpResp, err := a.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "anthropic request failed", Cause: err}}
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "failed to read response", Cause: err}}
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, llm.NewProviderError("anthropic", httpResp.StatusCode,
			fmt.Sprintf("anthropic API error: %s", string(respBody)), nil)
	}

	return a.parseResponse(respBody)
}

// anthropicRequest is the wire format for the Anthropic Messages API.
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      json.RawMessage    `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice `json:"tool_choice,omitempty"`
	Stop        []string           `json:"stop_sequences,omitempty"`
}

type anthropicMessage struct {
	Role    string            `json:"role"`
	Content json.RawMessage   `json:"content"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (a *Anthropic) buildRequest(req llm.Request) ([]byte, error) {
	ar := anthropicRequest{
		Model:       req.Model,
		MaxTokens:   anthropicDefaultMaxToks,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}
	if req.MaxTokens != nil {
		ar.MaxTokens = *req.MaxTokens
	}

	// Extract system messages.
	var systemParts []anthropicContentBlock
	var msgs []llm.Message
	for _, m := range req.Messages {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			systemParts = append(systemParts, anthropicContentBlock{
				Type: "text",
				Text: m.Text(),
			})
		} else {
			msgs = append(msgs, m)
		}
	}
	if len(systemParts) > 0 {
		b, _ := json.Marshal(systemParts)
		ar.System = b
	}

	// Convert messages with strict user/assistant alternation.
	for _, m := range msgs {
		am := anthropicMessage{Role: anthropicRole(m.Role)}
		blocks := a.contentToBlocks(m)
		b, _ := json.Marshal(blocks)
		am.Content = b
		ar.Messages = append(ar.Messages, am)
	}

	// Convert tools.
	for _, t := range req.Tools {
		ar.Tools = append(ar.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	// Convert tool choice.
	if req.ToolChoice != nil {
		tc := &anthropicToolChoice{}
		switch req.ToolChoice.Mode {
		case "auto":
			tc.Type = "auto"
		case "none":
			// Anthropic doesn't have "none" — omit tools instead.
			ar.Tools = nil
			tc = nil
		case "required":
			tc.Type = "any"
		case "named":
			tc.Type = "tool"
			tc.Name = req.ToolChoice.ToolName
		}
		ar.ToolChoice = tc
	}

	return json.Marshal(ar)
}

func (a *Anthropic) contentToBlocks(m llm.Message) []anthropicContentBlock {
	var blocks []anthropicContentBlock
	for _, p := range m.Content {
		switch p.Kind {
		case llm.ContentText:
			blocks = append(blocks, anthropicContentBlock{Type: "text", Text: p.Text})
		case llm.ContentToolCall:
			if p.ToolCall != nil {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    p.ToolCall.ID,
					Name:  p.ToolCall.Name,
					Input: p.ToolCall.Arguments,
				})
			}
		case llm.ContentToolResult:
			if p.ToolResult != nil {
				resultContent, _ := json.Marshal([]anthropicContentBlock{
					{Type: "text", Text: p.ToolResult.Content},
				})
				blocks = append(blocks, anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: p.ToolResult.ToolCallID,
					Content:   resultContent,
					IsError:   p.ToolResult.IsError,
				})
			}
		case llm.ContentThinking:
			if p.Thinking != nil {
				blocks = append(blocks, anthropicContentBlock{
					Type:      "thinking",
					Thinking:  p.Thinking.Text,
					Signature: p.Thinking.Signature,
				})
			}
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, anthropicContentBlock{Type: "text", Text: ""})
	}
	return blocks
}

func (a *Anthropic) parseResponse(body []byte) (*llm.Response, error) {
	var ar anthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, &llm.SDKError{Message: "failed to parse anthropic response", Cause: err}
	}

	msg := llm.Message{Role: llm.RoleAssistant}
	for _, block := range ar.Content {
		switch block.Type {
		case "text":
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentText,
				Text: block.Text,
			})
		case "tool_use":
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentToolCall,
				ToolCall: &llm.ToolCallData{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		case "thinking":
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentThinking,
				Thinking: &llm.ThinkingData{
					Text:      block.Thinking,
					Signature: block.Signature,
				},
			})
		}
	}

	finish := llm.FinishStop
	switch ar.StopReason {
	case "end_turn", "stop_sequence":
		finish = llm.FinishStop
	case "tool_use":
		finish = llm.FinishToolUse
	case "max_tokens":
		finish = llm.FinishLength
	}

	return &llm.Response{
		ID:           ar.ID,
		Model:        ar.Model,
		Provider:     "anthropic",
		Message:      msg,
		FinishReason: finish,
		Usage: llm.Usage{
			InputTokens:      ar.Usage.InputTokens,
			OutputTokens:     ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
			CacheReadTokens:  ar.Usage.CacheReadInputTokens,
			CacheWriteTokens: ar.Usage.CacheCreationInputTokens,
		},
		Raw: body,
	}, nil
}

func anthropicRole(r llm.Role) string {
	switch r {
	case llm.RoleAssistant:
		return "assistant"
	case llm.RoleTool:
		return "user"
	default:
		return "user"
	}
}

// SystemText is a helper to extract system text from messages.
func SystemText(messages []llm.Message) string {
	var parts []string
	for _, m := range messages {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			if t := m.Text(); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
