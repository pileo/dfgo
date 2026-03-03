package llm

import (
	"context"
	"fmt"
	"log/slog"
)

// Client is the unified LLM client that routes requests to provider adapters.
type Client struct {
	providers   map[string]ProviderAdapter
	defaultProv string
	middleware  []Middleware
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithProvider registers a provider adapter as the default (or additional) provider.
func WithProvider(adapter ProviderAdapter) ClientOption {
	return func(c *Client) {
		name := adapter.Name()
		c.providers[name] = adapter
		if c.defaultProv == "" {
			c.defaultProv = name
		}
	}
}

// WithDefaultProvider sets which provider to use when Request.Provider is empty.
func WithDefaultProvider(name string) ClientOption {
	return func(c *Client) {
		c.defaultProv = name
	}
}

// WithMiddleware adds middleware to the client. Middleware is applied in order
// (first added = outermost wrapper).
func WithMiddleware(mw ...Middleware) ClientOption {
	return func(c *Client) {
		c.middleware = append(c.middleware, mw...)
	}
}

// WithLogging adds debug-level request/response logging.
func WithLogging(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.middleware = append(c.middleware, LoggingMiddleware(logger))
	}
}

// WithRetry adds retry middleware with the given policy.
func WithRetry(policy RetryPolicy) ClientOption {
	return func(c *Client) {
		c.middleware = append(c.middleware, RetryMiddleware(policy))
	}
}

// NewClient creates a new unified LLM client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		providers: make(map[string]ProviderAdapter),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// RegisterProvider adds a provider adapter to the client.
func (c *Client) RegisterProvider(name string, adapter ProviderAdapter) {
	c.providers[name] = adapter
	if c.defaultProv == "" {
		c.defaultProv = name
	}
}

// Complete sends a completion request to the appropriate provider.
func (c *Client) Complete(ctx context.Context, req Request) (*Response, error) {
	provName := req.Provider
	if provName == "" {
		provName = c.defaultProv
	}
	if provName == "" {
		return nil, &SDKError{Message: "no provider specified and no default provider configured"}
	}

	adapter, ok := c.providers[provName]
	if !ok {
		return nil, &SDKError{Message: fmt.Sprintf("unknown provider: %q", provName)}
	}

	// Apply middleware (last added = innermost, first added = outermost).
	wrapped := adapter
	for i := len(c.middleware) - 1; i >= 0; i-- {
		wrapped = c.middleware[i](wrapped)
	}

	return wrapped.Complete(ctx, req)
}

// Stream sends a streaming completion request. If the provider supports
// streaming (implements StreamingProvider), it uses the native stream.
// Otherwise, it falls back to Complete() and wraps the result as a
// single-event stream.
func (c *Client) Stream(ctx context.Context, req Request) (*Stream, error) {
	provName := req.Provider
	if provName == "" {
		provName = c.defaultProv
	}
	if provName == "" {
		return nil, &SDKError{Message: "no provider specified and no default provider configured"}
	}

	adapter, ok := c.providers[provName]
	if !ok {
		return nil, &SDKError{Message: fmt.Sprintf("unknown provider: %q", provName)}
	}

	// Apply middleware (same as Complete).
	wrapped := adapter
	for i := len(c.middleware) - 1; i >= 0; i-- {
		wrapped = c.middleware[i](wrapped)
	}

	// Use native streaming if available.
	if sp, ok := wrapped.(StreamingProvider); ok {
		return sp.CompleteStream(ctx, req)
	}

	// Fallback: Complete() → synthesized stream.
	resp, err := wrapped.Complete(ctx, req)
	return CompleteToStream(resp, err), nil
}

// Close releases any resources held by the client.
func (c *Client) Close() {
	// Currently a no-op; providers don't hold persistent connections.
}
