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

	"github.com/google/uuid"
)

const (
	geminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

// Gemini implements the Google Gemini native API.
type Gemini struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewGemini creates a Gemini provider, reading API key from env if not set.
func NewGemini(opts ...func(*Gemini)) *Gemini {
	g := &Gemini{
		BaseURL: geminiDefaultBaseURL,
		Client:  http.DefaultClient,
	}
	for _, o := range opts {
		o(g)
	}
	if g.APIKey == "" {
		g.APIKey = os.Getenv("GEMINI_API_KEY")
	}
	return g
}

func (g *Gemini) Name() string { return "gemini" }

func (g *Gemini) Complete(ctx context.Context, req llm.Request) (*llm.Response, error) {
	body, err := g.buildRequest(req)
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to build gemini request", Cause: err}
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", g.BaseURL, req.Model, g.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to create http request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := g.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "gemini request failed", Cause: err}}
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "failed to read response", Cause: err}}
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, llm.NewProviderError("gemini", httpResp.StatusCode,
			fmt.Sprintf("gemini API error: %s", string(respBody)), nil)
	}

	return g.parseResponse(respBody)
}

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Tools             []geminiToolDecl        `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp   `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiToolDecl struct {
	FunctionDeclarations []geminiFunction `json:"functionDeclarations"`
}

type geminiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiToolConfig struct {
	FunctionCallingConfig *geminiFuncCallConfig `json:"functionCallingConfig,omitempty"`
}

type geminiFuncCallConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

func (g *Gemini) buildRequest(req llm.Request) ([]byte, error) {
	gr := geminiRequest{}

	// Generation config.
	gc := &geminiGenerationConfig{
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxOutputTokens: req.MaxTokens,
		StopSequences:   req.Stop,
	}
	gr.GenerationConfig = gc

	// Extract system instruction.
	sysText := SystemText(req.Messages)
	if sysText != "" {
		gr.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: sysText}},
		}
	}

	// Convert messages.
	for _, m := range req.Messages {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleDeveloper {
			continue
		}
		gc := geminiContent{Role: geminiRole(m.Role)}
		for _, p := range m.Content {
			switch p.Kind {
			case llm.ContentText:
				gc.Parts = append(gc.Parts, geminiPart{Text: p.Text})
			case llm.ContentToolCall:
				if p.ToolCall != nil {
					var args map[string]any
					_ = json.Unmarshal(p.ToolCall.Arguments, &args)
					gc.Parts = append(gc.Parts, geminiPart{
						FunctionCall: &geminiFunctionCall{
							Name: p.ToolCall.Name,
							Args: args,
						},
					})
				}
			case llm.ContentToolResult:
				if p.ToolResult != nil {
					gc.Parts = append(gc.Parts, geminiPart{
						FunctionResponse: &geminiFunctionResp{
							Name: p.ToolResult.ToolCallID, // Gemini uses function name, but we repurpose
							Response: map[string]any{
								"content":  p.ToolResult.Content,
								"is_error": p.ToolResult.IsError,
							},
						},
					})
				}
			}
		}
		if len(gc.Parts) > 0 {
			gr.Contents = append(gr.Contents, gc)
		}
	}

	// Convert tools.
	if len(req.Tools) > 0 {
		var funcs []geminiFunction
		for _, t := range req.Tools {
			funcs = append(funcs, geminiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
		gr.Tools = []geminiToolDecl{{FunctionDeclarations: funcs}}
	}

	// Convert tool choice.
	if req.ToolChoice != nil {
		config := &geminiFuncCallConfig{}
		switch req.ToolChoice.Mode {
		case "auto":
			config.Mode = "AUTO"
		case "none":
			config.Mode = "NONE"
		case "required":
			config.Mode = "ANY"
		case "named":
			config.Mode = "ANY"
			config.AllowedFunctionNames = []string{req.ToolChoice.ToolName}
		}
		gr.ToolConfig = &geminiToolConfig{FunctionCallingConfig: config}
	}

	return json.Marshal(gr)
}

func (g *Gemini) parseResponse(body []byte) (*llm.Response, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, &llm.SDKError{Message: "failed to parse gemini response", Cause: err}
	}

	if len(gr.Candidates) == 0 {
		return nil, &llm.SDKError{Message: "gemini returned no candidates"}
	}

	cand := gr.Candidates[0]
	msg := llm.Message{Role: llm.RoleAssistant}

	for _, part := range cand.Content.Parts {
		if part.Text != "" {
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentText,
				Text: part.Text,
			})
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			msg.Content = append(msg.Content, llm.ContentPart{
				Kind: llm.ContentToolCall,
				ToolCall: &llm.ToolCallData{
					ID:        uuid.New().String(), // Gemini has no native tool call IDs
					Name:      part.FunctionCall.Name,
					Arguments: args,
				},
			})
		}
	}

	finish := llm.FinishStop
	switch cand.FinishReason {
	case "STOP":
		if len(msg.ToolCalls()) > 0 {
			finish = llm.FinishToolUse
		}
	case "MAX_TOKENS":
		finish = llm.FinishLength
	case "SAFETY":
		finish = llm.FinishContentFilter
	}

	return &llm.Response{
		ID:       "",
		Model:    "",
		Provider: "gemini",
		Message:  msg,
		FinishReason: finish,
		Usage: llm.Usage{
			InputTokens:  gr.UsageMetadata.PromptTokenCount,
			OutputTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  gr.UsageMetadata.TotalTokenCount,
		},
		Raw: body,
	}, nil
}

func geminiRole(r llm.Role) string {
	switch r {
	case llm.RoleAssistant:
		return "model"
	default:
		return "user"
	}
}
