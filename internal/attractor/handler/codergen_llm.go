package handler

import (
	"context"

	"dfgo/internal/llm"
)

// LLMCodergenBackend implements CodergenBackend using the unified LLM client.
// This allows one-shot LLM calls in codergen stages to go through the same
// provider infrastructure as the coding agent.
type LLMCodergenBackend struct {
	Client *llm.Client
	Model  string
}

// NewLLMCodergenBackend creates a CodergenBackend backed by an llm.Client.
func NewLLMCodergenBackend(client *llm.Client, model string) *LLMCodergenBackend {
	return &LLMCodergenBackend{Client: client, Model: model}
}

func (b *LLMCodergenBackend) Generate(ctx context.Context, prompt string, opts map[string]string) (string, error) {
	model := b.Model
	if m, ok := opts["model"]; ok && m != "" {
		model = m
	}

	req := llm.Request{
		Model: model,
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, prompt),
		},
	}

	// Pass through provider option if specified.
	if p, ok := opts["provider"]; ok && p != "" {
		req.Provider = p
	}

	resp, err := b.Client.Complete(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Text(), nil
}
