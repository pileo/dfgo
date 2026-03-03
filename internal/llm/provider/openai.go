package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"dfgo/internal/llm"
)

const (
	openaiDefaultURL = "https://api.openai.com/v1/responses"
)

// OpenAI implements the OpenAI Responses API.
type OpenAI struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewOpenAI creates an OpenAI provider, reading API key from env if not set.
func NewOpenAI(opts ...func(*OpenAI)) *OpenAI {
	o := &OpenAI{
		BaseURL: openaiDefaultURL,
		Client:  http.DefaultClient,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.APIKey == "" {
		o.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	return o
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	body, err := o.buildRequest(req)
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to build openai request", Cause: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to create http request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)

	httpResp, err := o.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "openai request failed", Cause: err}}
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "failed to read response", Cause: err}}
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, llm.NewProviderError("openai", httpResp.StatusCode,
			fmt.Sprintf("openai API error: %s", string(respBody)), nil)
	}

	return o.parseResponse(respBody)
}

type openaiRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions,omitempty"`
	Tools        []openaiTool    `json:"tools,omitempty"`
	ToolChoice   any             `json:"tool_choice,omitempty"`
	Temperature  *float64        `json:"temperature,omitempty"`
	MaxTokens    *int            `json:"max_output_tokens,omitempty"`
	Reasoning    *openaiReasoning `json:"reasoning,omitempty"`
}

type openaiReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openaiTool struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Description string       `json:"description,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

type openaiInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Output  []openaiOutput   `json:"output"`
	Model   string           `json:"model"`
	Usage   openaiUsage      `json:"usage"`
	Status  string           `json:"status"`
}

type openaiOutput struct {
	Type      string             `json:"type"`
	ID        string             `json:"id"`
	Content   []openaiContent    `json:"content,omitempty"`
	Name      string             `json:"name,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
	Status    string             `json:"status,omitempty"`
	Summary   []openaiContent    `json:"summary,omitempty"`
}

type openaiContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type openaiUsage struct {
	InputTokens          int `json:"input_tokens"`
	OutputTokens         int `json:"output_tokens"`
	TotalTokens          int `json:"total_tokens"`
	OutputTokensDetails  struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (o *OpenAI) buildRequest(req llm.Request) ([]byte, error) {
	or := openaiRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	if req.ReasoningEffort != "" {
		or.Reasoning = &openaiReasoning{Effort: req.ReasoningEffort}
	}

	// Extract system messages as instructions.
	sysText := SystemText(req.Messages)
	if sysText != "" {
		or.Instructions = sysText
	}

	// Convert non-system messages to input items.
	var inputMsgs []openaiInputMessage
	for _, m := range req.Messages {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			continue
		}
		inputMsgs = append(inputMsgs, openaiInputMessage{
			Role:    string(m.Role),
			Content: m.Text(),
		})
	}
	inputJSON, _ := json.Marshal(inputMsgs)
	or.Input = inputJSON

	// Convert tools.
	for _, t := range req.Tools {
		or.Tools = append(or.Tools, openaiTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}

	// Convert tool choice.
	if req.ToolChoice != nil {
		switch req.ToolChoice.Mode {
		case "auto":
			or.ToolChoice = "auto"
		case "none":
			or.ToolChoice = "none"
		case "required":
			or.ToolChoice = "required"
		case "named":
			or.ToolChoice = map[string]string{"type": "function", "name": req.ToolChoice.ToolName}
		}
	}

	return json.Marshal(or)
}

func (o *OpenAI) parseResponse(body []byte) (*llm.Response, error) {
	var or openaiResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, &llm.SDKError{Message: "failed to parse openai response", Cause: err}
	}

	msg := llm.Message{Role: llm.RoleAssistant}
	for _, output := range or.Output {
		switch output.Type {
		case "message":
			for _, c := range output.Content {
				if c.Type == "output_text" || c.Type == "text" {
					msg.Content = append(msg.Content, llm.ContentPart{
						Kind: llm.ContentText,
						Text: c.Text,
					})
				}
			}
		case "function_call":
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentToolCall,
				ToolCall: &llm.ToolCallData{
					ID:        output.CallID,
					Name:      output.Name,
					Arguments: json.RawMessage(output.Arguments),
				},
			})
		case "reasoning":
			for _, s := range output.Summary {
				msg.Content = append(msg.Content, llm.ContentPart{
					Kind: llm.ContentThinking,
					Thinking: &llm.ThinkingData{
						Text: s.Text,
					},
				})
			}
		}
	}

	finish := llm.FinishStop
	switch or.Status {
	case "completed":
		// Check if there are function calls.
		if len(msg.ToolCalls()) > 0 {
			finish = llm.FinishToolUse
		}
	case "incomplete":
		finish = llm.FinishLength
	}

	return &llm.Response{
		ID:           or.ID,
		Model:        or.Model,
		Provider:     "openai",
		Message:      msg,
		FinishReason: finish,
		Usage: llm.Usage{
			InputTokens:    or.Usage.InputTokens,
			OutputTokens:   or.Usage.OutputTokens,
			TotalTokens:    or.Usage.TotalTokens,
			ReasoningTokens: or.Usage.OutputTokensDetails.ReasoningTokens,
		},
		Raw: body,
	}, nil
}
