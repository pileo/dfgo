package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"dfgo/internal/llm"
)

// CompleteStream sends a streaming completion request to the Anthropic Messages API.
func (a *Anthropic) CompleteStream(ctx context.Context, req llm.Request) (*llm.Stream, error) {
	body, err := a.buildStreamRequest(req)
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to build anthropic stream request", Cause: err}
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
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "anthropic stream request failed", Cause: err}}
	}

	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, llm.NewProviderError("anthropic", httpResp.StatusCode,
			fmt.Sprintf("anthropic API error (stream): status %d", httpResp.StatusCode), nil)
	}

	stream := llm.NewStream(ctx, httpResp.Body, 64)
	go a.readStream(stream)
	return stream, nil
}

// buildStreamRequest is like buildRequest but adds "stream": true.
func (a *Anthropic) buildStreamRequest(req llm.Request) ([]byte, error) {
	base, err := a.buildRequest(req)
	if err != nil {
		return nil, err
	}
	// Inject "stream": true into the JSON.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	m["stream"] = json.RawMessage(`true`)
	return json.Marshal(m)
}

// Anthropic SSE event types.
type anthropicSSEMessageStart struct {
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthropicSSEContentBlockStart struct {
	Index        int                    `json:"index"`
	ContentBlock anthropicContentBlock  `json:"content_block"`
}

type anthropicSSEContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type            string `json:"type"`
		Text            string `json:"text,omitempty"`
		PartialJSON     string `json:"partial_json,omitempty"`
		Thinking        string `json:"thinking,omitempty"`
	} `json:"delta"`
}

type anthropicSSEContentBlockStop struct {
	Index int `json:"index"`
}

type anthropicSSEMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// readStream reads SSE events from the Anthropic API and translates them to
// unified StreamEvents. It accumulates a full Response for stream.Response().
func (a *Anthropic) readStream(stream *llm.Stream) {
	scanner := llm.NewSSEScanner(stream.Body())

	resp := &llm.Response{
		Provider: "anthropic",
		Message:  llm.Message{Role: llm.RoleAssistant},
	}
	var usage llm.Usage

	// Track content blocks being built for accumulation.
	type blockState struct {
		kind      llm.ContentKind
		text      strings.Builder
		toolID    string
		toolName  string
		signature string
	}
	blocks := make(map[int]*blockState)

	for scanner.Next() {
		sse := scanner.Event()

		switch sse.Event {
		case "message_start":
			var ms anthropicSSEMessageStart
			if json.Unmarshal([]byte(sse.Data), &ms) != nil {
				continue
			}
			resp.ID = ms.Message.ID
			resp.Model = ms.Message.Model
			usage.InputTokens = ms.Message.Usage.InputTokens
			usage.CacheReadTokens = ms.Message.Usage.CacheReadInputTokens
			usage.CacheWriteTokens = ms.Message.Usage.CacheCreationInputTokens

			if !stream.Send(llm.StreamEvent{
				Type:       llm.EventResponseMeta,
				ResponseID: resp.ID,
				Model:      resp.Model,
			}) {
				return
			}

		case "content_block_start":
			var cbs anthropicSSEContentBlockStart
			if json.Unmarshal([]byte(sse.Data), &cbs) != nil {
				continue
			}
			bs := &blockState{}
			var evt llm.StreamEvent
			evt.Type = llm.EventContentStart
			evt.Index = cbs.Index

			switch cbs.ContentBlock.Type {
			case "text":
				bs.kind = llm.ContentText
				evt.ContentKind = llm.ContentText
			case "tool_use":
				bs.kind = llm.ContentToolCall
				bs.toolID = cbs.ContentBlock.ID
				bs.toolName = cbs.ContentBlock.Name
				evt.ContentKind = llm.ContentToolCall
				evt.ToolCallID = cbs.ContentBlock.ID
				evt.ToolName = cbs.ContentBlock.Name
			case "thinking":
				bs.kind = llm.ContentThinking
				evt.ContentKind = llm.ContentThinking
			}
			blocks[cbs.Index] = bs

			if !stream.Send(evt) {
				return
			}

		case "content_block_delta":
			var cbd anthropicSSEContentBlockDelta
			if json.Unmarshal([]byte(sse.Data), &cbd) != nil {
				continue
			}
			bs := blocks[cbd.Index]
			if bs == nil {
				continue
			}

			var text string
			switch cbd.Delta.Type {
			case "text_delta":
				text = cbd.Delta.Text
			case "input_json_delta":
				text = cbd.Delta.PartialJSON
			case "thinking_delta":
				text = cbd.Delta.Thinking
			case "signature_delta":
				// Signature deltas accumulate but aren't text content.
				bs.signature += cbd.Delta.Text
				continue
			}
			bs.text.WriteString(text)

			if !stream.Send(llm.StreamEvent{
				Type:        llm.EventContentDelta,
				Index:       cbd.Index,
				ContentKind: bs.kind,
				Text:        text,
			}) {
				return
			}

		case "content_block_stop":
			var cbstop anthropicSSEContentBlockStop
			if json.Unmarshal([]byte(sse.Data), &cbstop) != nil {
				continue
			}
			bs := blocks[cbstop.Index]
			if bs == nil {
				continue
			}

			// Accumulate the completed block into the response.
			switch bs.kind {
			case llm.ContentText:
				resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
					Kind: llm.ContentText,
					Text: bs.text.String(),
				})
			case llm.ContentToolCall:
				resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
					Kind: llm.ContentToolCall,
					ToolCall: &llm.ToolCallData{
						ID:        bs.toolID,
						Name:      bs.toolName,
						Arguments: json.RawMessage(bs.text.String()),
					},
				})
			case llm.ContentThinking:
				resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
					Kind: llm.ContentThinking,
					Thinking: &llm.ThinkingData{
						Text:      bs.text.String(),
						Signature: bs.signature,
					},
				})
			}

			if !stream.Send(llm.StreamEvent{
				Type:        llm.EventContentStop,
				Index:       cbstop.Index,
				ContentKind: bs.kind,
			}) {
				return
			}

		case "message_delta":
			var md anthropicSSEMessageDelta
			if json.Unmarshal([]byte(sse.Data), &md) != nil {
				continue
			}
			usage.OutputTokens = md.Usage.OutputTokens
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens

			finish := llm.FinishStop
			switch md.Delta.StopReason {
			case "end_turn", "stop_sequence":
				finish = llm.FinishStop
			case "tool_use":
				finish = llm.FinishToolUse
			case "max_tokens":
				finish = llm.FinishLength
			}
			resp.FinishReason = finish
			resp.Usage = usage

			if !stream.Send(llm.StreamEvent{
				Type:         llm.EventUsage,
				Usage:        &usage,
				FinishReason: finish,
			}) {
				return
			}

		case "message_stop":
			stream.Finish(resp, nil)
			return

		case "error":
			var errEvt struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(sse.Data), &errEvt) != nil {
				stream.Finish(nil, &llm.SDKError{Message: "anthropic stream error: " + sse.Data})
				return
			}
			stream.Send(llm.StreamEvent{
				Type: llm.EventError,
				Err:  &llm.SDKError{Message: fmt.Sprintf("anthropic stream error: %s: %s", errEvt.Error.Type, errEvt.Error.Message)},
			})
			stream.Finish(nil, &llm.SDKError{
				Message: fmt.Sprintf("anthropic stream error: %s: %s", errEvt.Error.Type, errEvt.Error.Message),
			})
			return

		case "ping":
			// Keepalive, ignore.
		}
	}

	// Stream ended without message_stop — could be a disconnect.
	if err := scanner.Err(); err != nil {
		stream.Finish(nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "anthropic stream interrupted", Cause: err}})
		return
	}
	// Normal EOF without message_stop — treat as unexpected.
	stream.Finish(nil, &llm.SDKError{Message: "anthropic stream ended without message_stop"})
}
