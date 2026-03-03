package llm

import (
	"errors"
	"fmt"
)

// SDKError is the base error type for all LLM SDK errors.
type SDKError struct {
	Message string
	Cause   error
}

func (e *SDKError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *SDKError) Unwrap() error { return e.Cause }

// ProviderError indicates a provider-level error (HTTP response from the API).
type ProviderError struct {
	SDKError
	Provider   string
	StatusCode int
	Retryable  bool
	RetryAfter float64 // seconds; 0 if not specified
}

// AuthenticationError indicates invalid or missing API credentials.
type AuthenticationError struct{ ProviderError }

// RateLimitError indicates the request was rate-limited.
type RateLimitError struct{ ProviderError }

// ServerError indicates a provider-side server error (5xx).
type ServerError struct{ ProviderError }

// ContextLengthError indicates the request exceeded the model's context window.
type ContextLengthError struct{ ProviderError }

// InvalidRequestError indicates the request was malformed.
type InvalidRequestError struct{ ProviderError }

// ContentFilterError indicates the request or response was blocked by content filters.
type ContentFilterError struct{ ProviderError }

// QuotaExceededError indicates the account has exceeded its quota.
type QuotaExceededError struct{ ProviderError }

// RequestTimeoutError indicates the request timed out.
type RequestTimeoutError struct{ SDKError }

// AbortError indicates the request was explicitly aborted (e.g., via context cancellation).
type AbortError struct{ SDKError }

// NetworkError indicates a network-level failure.
type NetworkError struct{ SDKError }

// IsRetryable checks the error chain for retryable errors.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Retryable
	}
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return true
	}
	var se *ServerError
	if errors.As(err, &se) {
		return true
	}
	var ne *NetworkError
	if errors.As(err, &ne) {
		return true
	}
	var te *RequestTimeoutError
	if errors.As(err, &te) {
		return true
	}
	return false
}

// NewProviderError creates a ProviderError classified by HTTP status code.
func NewProviderError(provider string, statusCode int, message string, cause error) error {
	base := ProviderError{
		SDKError:   SDKError{Message: message, Cause: cause},
		Provider:   provider,
		StatusCode: statusCode,
	}
	switch {
	case statusCode == 401 || statusCode == 403:
		return &AuthenticationError{ProviderError: base}
	case statusCode == 429:
		base.Retryable = true
		return &RateLimitError{ProviderError: base}
	case statusCode == 400:
		return &InvalidRequestError{ProviderError: base}
	case statusCode == 413:
		return &ContextLengthError{ProviderError: base}
	case statusCode >= 500:
		base.Retryable = true
		return &ServerError{ProviderError: base}
	default:
		return &base
	}
}
