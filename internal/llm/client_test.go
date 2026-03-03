package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
)

// mockProvider is a test double for ProviderAdapter.
type mockProvider struct {
	name     string
	response *Response
	err      error
	calls    int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Complete(_ context.Context, _ Request) (*Response, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestClientComplete(t *testing.T) {
	resp := &Response{
		ID:           "resp-1",
		Model:        "test-model",
		Provider:     "test",
		Message:      TextMessage(RoleAssistant, "hello"),
		FinishReason: FinishStop,
	}
	prov := &mockProvider{name: "test", response: resp}
	c := NewClient(WithProvider(prov))

	got, err := c.Complete(context.Background(), Request{Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text() != "hello" {
		t.Errorf("got %q, want %q", got.Text(), "hello")
	}
	if prov.calls != 1 {
		t.Errorf("calls = %d, want 1", prov.calls)
	}
}

func TestClientExplicitProvider(t *testing.T) {
	prov1 := &mockProvider{name: "a", response: &Response{Provider: "a", Message: TextMessage(RoleAssistant, "from-a")}}
	prov2 := &mockProvider{name: "b", response: &Response{Provider: "b", Message: TextMessage(RoleAssistant, "from-b")}}
	c := NewClient(WithProvider(prov1), WithProvider(prov2))

	got, err := c.Complete(context.Background(), Request{Provider: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "b" {
		t.Errorf("provider = %q, want %q", got.Provider, "b")
	}
}

func TestClientNoProvider(t *testing.T) {
	c := NewClient()
	_, err := c.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error for no provider")
	}
}

func TestClientUnknownProvider(t *testing.T) {
	prov := &mockProvider{name: "test"}
	c := NewClient(WithProvider(prov))
	_, err := c.Complete(context.Background(), Request{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestClientMiddleware(t *testing.T) {
	prov := &mockProvider{
		name:     "test",
		response: &Response{Message: TextMessage(RoleAssistant, "ok")},
	}
	c := NewClient(
		WithProvider(prov),
		WithLogging(slog.Default()),
	)
	_, err := c.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRetryMiddleware(t *testing.T) {
	// Provider fails twice then succeeds.
	call := 0
	prov := &mockProvider{name: "test"}
	retryable := &ServerError{ProviderError: ProviderError{
		SDKError:   SDKError{Message: "server error"},
		StatusCode: 500,
		Retryable:  true,
	}}
	okResp := &Response{Message: TextMessage(RoleAssistant, "ok")}

	wrapper := RetryMiddleware(RetryPolicy{MaxRetries: 3, BaseDelay: 0, MaxDelay: 0})
	adapted := wrapper(&customMock{name: "test", fn: func(ctx context.Context, req Request) (*Response, error) {
		call++
		if call < 3 {
			return nil, retryable
		}
		return okResp, nil
	}})

	resp, err := adapted.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != "ok" {
		t.Errorf("got %q, want %q", resp.Text(), "ok")
	}
	if call != 3 {
		t.Errorf("calls = %d, want 3", call)
	}
	_ = prov // suppress unused
}

func TestRetryNonRetryable(t *testing.T) {
	nonRetryable := &AuthenticationError{ProviderError: ProviderError{
		SDKError:   SDKError{Message: "auth error"},
		StatusCode: 401,
	}}

	call := 0
	wrapper := RetryMiddleware(RetryPolicy{MaxRetries: 3, BaseDelay: 0})
	adapted := wrapper(&customMock{name: "test", fn: func(ctx context.Context, req Request) (*Response, error) {
		call++
		return nil, nonRetryable
	}})

	_, err := adapted.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if call != 1 {
		t.Errorf("calls = %d, want 1 (should not retry)", call)
	}
}

// customMock allows inline function-based mocking.
type customMock struct {
	name string
	fn   func(context.Context, Request) (*Response, error)
}

func (m *customMock) Name() string { return m.name }
func (m *customMock) Complete(ctx context.Context, req Request) (*Response, error) {
	return m.fn(ctx, req)
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate limit", &RateLimitError{ProviderError: ProviderError{Retryable: true}}, true},
		{"server error", &ServerError{ProviderError: ProviderError{Retryable: true}}, true},
		{"auth error", &AuthenticationError{}, false},
		{"network error", &NetworkError{}, true},
		{"timeout", &RequestTimeoutError{}, true},
		{"generic", errors.New("generic"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewProviderError(t *testing.T) {
	tests := []struct {
		status int
		errTyp string
	}{
		{401, "*llm.AuthenticationError"},
		{403, "*llm.AuthenticationError"},
		{429, "*llm.RateLimitError"},
		{400, "*llm.InvalidRequestError"},
		{413, "*llm.ContextLengthError"},
		{500, "*llm.ServerError"},
		{502, "*llm.ServerError"},
		{418, "*llm.ProviderError"},
	}
	for _, tt := range tests {
		err := NewProviderError("test", tt.status, "msg", nil)
		got := fmt.Sprintf("%T", err)
		if got != tt.errTyp {
			t.Errorf("status %d: got %s, want %s", tt.status, got, tt.errTyp)
		}
	}
}

func TestMessageText(t *testing.T) {
	m := Message{
		Role: RoleAssistant,
		Content: []ContentPart{
			{Kind: ContentText, Text: "hello "},
			{Kind: ContentThinking, Thinking: &ThinkingData{Text: "thinking..."}},
			{Kind: ContentText, Text: "world"},
		},
	}
	if got := m.Text(); got != "hello world" {
		t.Errorf("Text() = %q, want %q", got, "hello world")
	}
}

// --- Stream tests ---

func TestClientStreamFallback(t *testing.T) {
	// Provider doesn't implement StreamingProvider — should fall back to Complete.
	resp := &Response{
		ID:           "resp-1",
		Model:        "test-model",
		Provider:     "test",
		Message:      TextMessage(RoleAssistant, "hello"),
		FinishReason: FinishStop,
		Usage:        Usage{InputTokens: 10, OutputTokens: 5},
	}
	prov := &mockProvider{name: "test", response: resp}
	c := NewClient(WithProvider(prov))

	stream, err := c.Stream(context.Background(), Request{Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}

	var events []StreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}

	if len(events) == 0 {
		t.Fatal("expected events from fallback stream")
	}

	got := stream.Response()
	if got.Text() != "hello" {
		t.Errorf("text = %q", got.Text())
	}
}

// streamingMock implements both ProviderAdapter and StreamingProvider.
type streamingMock struct {
	name       string
	response   *Response
	streamUsed bool
}

func (m *streamingMock) Name() string { return m.name }
func (m *streamingMock) Complete(_ context.Context, _ Request) (*Response, error) {
	return m.response, nil
}
func (m *streamingMock) CompleteStream(_ context.Context, _ Request) (*Stream, error) {
	m.streamUsed = true
	return CompleteToStream(m.response, nil), nil
}

func TestClientStreamNative(t *testing.T) {
	resp := &Response{
		ID:      "resp-s",
		Model:   "test",
		Message: TextMessage(RoleAssistant, "streamed"),
	}
	prov := &streamingMock{name: "test", response: resp}
	c := NewClient(WithProvider(prov))

	stream, err := c.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
	}

	if !prov.streamUsed {
		t.Error("expected CompleteStream to be called")
	}
}

func TestClientStreamWithMiddleware(t *testing.T) {
	resp := &Response{
		ID:      "resp-mw",
		Model:   "test",
		Message: TextMessage(RoleAssistant, "via middleware"),
	}
	prov := &streamingMock{name: "test", response: resp}
	c := NewClient(
		WithProvider(prov),
		WithLogging(slog.Default()),
	)

	stream, err := c.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}

	if !prov.streamUsed {
		t.Error("expected CompleteStream to be used through middleware")
	}
}

func TestClientStreamRetryMiddleware(t *testing.T) {
	retryable := &ServerError{ProviderError: ProviderError{
		SDKError:   SDKError{Message: "server error"},
		StatusCode: 500,
		Retryable:  true,
	}}

	call := 0
	resp := &Response{Message: TextMessage(RoleAssistant, "ok")}

	// streamingCustomMock implements StreamingProvider.
	type streamingCustom struct {
		customMock
	}
	mock := &struct {
		streamingCustom
	}{}
	mock.name = "test"
	mock.fn = func(ctx context.Context, req Request) (*Response, error) {
		return resp, nil
	}

	// Use a custom wrapper instead to test retry on CompleteStream.
	wrapper := RetryMiddleware(RetryPolicy{MaxRetries: 3, BaseDelay: 0, MaxDelay: 0})
	inner := &streamRetryMock{
		name: "test",
		fn: func(ctx context.Context, req Request) (*Stream, error) {
			call++
			if call < 3 {
				return nil, retryable
			}
			return CompleteToStream(resp, nil), nil
		},
	}
	adapted := wrapper(inner)

	sp, ok := adapted.(StreamingProvider)
	if !ok {
		t.Fatal("retry adapter should implement StreamingProvider when inner does")
	}

	stream, err := sp.CompleteStream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}

	for stream.Next() {
	}
	if call != 3 {
		t.Errorf("calls = %d, want 3", call)
	}
}

// streamRetryMock implements StreamingProvider for retry tests.
type streamRetryMock struct {
	name string
	fn   func(context.Context, Request) (*Stream, error)
}

func (m *streamRetryMock) Name() string { return m.name }
func (m *streamRetryMock) Complete(ctx context.Context, req Request) (*Response, error) {
	return nil, errors.New("use CompleteStream")
}
func (m *streamRetryMock) CompleteStream(ctx context.Context, req Request) (*Stream, error) {
	return m.fn(ctx, req)
}

func TestClientStreamNoProvider(t *testing.T) {
	c := NewClient()
	_, err := c.Stream(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error for no provider")
	}
}

func TestMessageToolCalls(t *testing.T) {
	m := Message{
		Role: RoleAssistant,
		Content: []ContentPart{
			{Kind: ContentText, Text: "I'll call a tool"},
			{Kind: ContentToolCall, ToolCall: &ToolCallData{ID: "tc1", Name: "read_file"}},
			{Kind: ContentToolCall, ToolCall: &ToolCallData{ID: "tc2", Name: "shell"}},
		},
	}
	calls := m.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(calls))
	}
	if calls[0].Name != "read_file" {
		t.Errorf("calls[0].Name = %q, want %q", calls[0].Name, "read_file")
	}
}
