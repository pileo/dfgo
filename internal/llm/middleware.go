package llm

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

// Middleware wraps a ProviderAdapter to add cross-cutting behavior.
type Middleware func(ProviderAdapter) ProviderAdapter

// RetryPolicy configures retry behavior for transient failures.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Jitter     bool
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   30 * time.Second,
		Jitter:     true,
	}
}

// LoggingMiddleware logs each request/response at debug level.
func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next ProviderAdapter) ProviderAdapter {
		return &loggingAdapter{next: next, logger: logger}
	}
}

type loggingAdapter struct {
	next   ProviderAdapter
	logger *slog.Logger
}

func (a *loggingAdapter) Name() string { return a.next.Name() }

func (a *loggingAdapter) Complete(ctx context.Context, req Request) (*Response, error) {
	a.logger.DebugContext(ctx, "llm request",
		"provider", a.next.Name(),
		"model", req.Model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
	)
	start := time.Now()
	resp, err := a.next.Complete(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		a.logger.WarnContext(ctx, "llm error",
			"provider", a.next.Name(),
			"elapsed", elapsed,
			"error", err,
		)
		return nil, err
	}
	a.logger.DebugContext(ctx, "llm response",
		"provider", a.next.Name(),
		"model", resp.Model,
		"finish_reason", resp.FinishReason,
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"elapsed", elapsed,
	)
	return resp, nil
}

// CompleteStream delegates to the inner adapter's CompleteStream if available,
// logging the stream start and relying on the caller to observe completion.
func (a *loggingAdapter) CompleteStream(ctx context.Context, req Request) (*Stream, error) {
	sp, ok := a.next.(StreamingProvider)
	if !ok {
		// Fallback: Complete + wrap.
		resp, err := a.Complete(ctx, req)
		return CompleteToStream(resp, err), nil
	}
	a.logger.DebugContext(ctx, "llm stream request",
		"provider", a.next.Name(),
		"model", req.Model,
		"messages", len(req.Messages),
		"tools", len(req.Tools),
	)
	stream, err := sp.CompleteStream(ctx, req)
	if err != nil {
		a.logger.WarnContext(ctx, "llm stream error",
			"provider", a.next.Name(),
			"error", err,
		)
	}
	return stream, err
}

// RetryMiddleware retries transient failures with exponential backoff.
func RetryMiddleware(policy RetryPolicy) Middleware {
	return func(next ProviderAdapter) ProviderAdapter {
		return &retryAdapter{next: next, policy: policy}
	}
}

type retryAdapter struct {
	next   ProviderAdapter
	policy RetryPolicy
}

func (a *retryAdapter) Name() string { return a.next.Name() }

func (a *retryAdapter) Complete(ctx context.Context, req Request) (*Response, error) {
	var lastErr error
	for attempt := 0; attempt <= a.policy.MaxRetries; attempt++ {
		resp, err := a.next.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !IsRetryable(err) {
			return nil, err
		}
		if attempt == a.policy.MaxRetries {
			break
		}
		delay := a.backoff(attempt)
		select {
		case <-ctx.Done():
			return nil, &AbortError{SDKError{Message: "request aborted during retry", Cause: ctx.Err()}}
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

// CompleteStream retries stream creation on connection errors (before first event).
// Once the stream is open, no retry is possible.
func (a *retryAdapter) CompleteStream(ctx context.Context, req Request) (*Stream, error) {
	sp, ok := a.next.(StreamingProvider)
	if !ok {
		// Fallback: use Complete with retry, then wrap.
		resp, err := a.Complete(ctx, req)
		return CompleteToStream(resp, err), nil
	}

	var lastErr error
	for attempt := 0; attempt <= a.policy.MaxRetries; attempt++ {
		stream, err := sp.CompleteStream(ctx, req)
		if err == nil {
			return stream, nil
		}
		lastErr = err
		if !IsRetryable(err) {
			return nil, err
		}
		if attempt == a.policy.MaxRetries {
			break
		}
		delay := a.backoff(attempt)
		select {
		case <-ctx.Done():
			return nil, &AbortError{SDKError{Message: "stream request aborted during retry", Cause: ctx.Err()}}
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func (a *retryAdapter) backoff(attempt int) time.Duration {
	delay := float64(a.policy.BaseDelay) * math.Pow(2, float64(attempt))
	if delay > float64(a.policy.MaxDelay) {
		delay = float64(a.policy.MaxDelay)
	}
	if a.policy.Jitter {
		delay = delay * (0.5 + rand.Float64()*0.5)
	}
	return time.Duration(delay)
}
