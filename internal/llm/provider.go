package llm

import "context"

// ProviderAdapter translates between the unified Request/Response types
// and a specific LLM provider's API.
type ProviderAdapter interface {
	// Name returns the provider identifier (e.g., "anthropic", "openai", "gemini").
	Name() string
	// Complete sends a completion request and returns the response.
	Complete(ctx context.Context, req Request) (*Response, error)
}

// StreamingProvider is an optional extension of ProviderAdapter for providers
// that support streaming responses. Providers that don't implement this
// interface fall back to Complete() with a synthesized single-event stream.
type StreamingProvider interface {
	ProviderAdapter
	// CompleteStream sends a completion request and returns a streaming response.
	CompleteStream(ctx context.Context, req Request) (*Stream, error)
}
