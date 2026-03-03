package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"dfgo/internal/llm"
)

// CompleteStream sends a streaming completion request to the OpenAI Responses API.
func (o *OpenAI) CompleteStream(ctx context.Context, req llm.Request) (*llm.Stream, error) {
	body, err := o.buildStreamRequest(req)
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to build openai stream request", Cause: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to create http request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)

	httpResp, err := o.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "openai stream request failed", Cause: err}}
	}

	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, llm.NewProviderError("openai", httpResp.StatusCode,
			fmt.Sprintf("openai API error (stream): status %d", httpResp.StatusCode), nil)
	}

	stream := llm.NewStream(ctx, httpResp.Body, 64)
	go o.readStream(stream)
	return stream, nil
}

// buildStreamRequest adds "stream": true to the request.
func (o *OpenAI) buildStreamRequest(req llm.Request) ([]byte, error) {
	base, err := o.buildRequest(req)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	m["stream"] = json.RawMessage(`true`)
	return json.Marshal(m)
}

// OpenAI Responses API streaming event types.
type openaiSSEResponse struct {
	ID    string      `json:"id"`
	Model string      `json:"model"`
	Usage *openaiUsage `json:"usage,omitempty"`
}

type openaiSSEOutputItem struct {
	ItemID string `json:"item_id"`
	Index  int    `json:"output_index"`
	Item   struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Name   string `json:"name,omitempty"`
		CallID string `json:"call_id,omitempty"`
	} `json:"item"`
}

type openaiSSEContentPart struct {
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Part         struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"part"`
}

type openaiSSETextDelta struct {
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type openaiSSEFuncArgsDelta struct {
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

type openaiSSEOutputItemDone struct {
	Item openaiOutput `json:"item"`
}

// readStream reads SSE events from the OpenAI Responses API.
func (o *OpenAI) readStream(stream *llm.Stream) {
	scanner := llm.NewSSEScanner(stream.Body())

	resp := &llm.Response{
		Provider: "openai",
		Message:  llm.Message{Role: llm.RoleAssistant},
	}

	// Track output items for index mapping.
	type itemState struct {
		kind     llm.ContentKind
		itemType string
		callID   string
		name     string
	}
	items := make(map[int]*itemState)

	for scanner.Next() {
		sse := scanner.Event()

		switch sse.Event {
		case "response.created":
			var rc openaiSSEResponse
			if json.Unmarshal([]byte(sse.Data), &rc) != nil {
				continue
			}
			resp.ID = rc.ID
			resp.Model = rc.Model

			if !stream.Send(llm.StreamEvent{
				Type:       llm.EventResponseMeta,
				ResponseID: rc.ID,
				Model:      rc.Model,
			}) {
				return
			}

		case "response.output_item.added":
			var oi openaiSSEOutputItem
			if json.Unmarshal([]byte(sse.Data), &oi) != nil {
				continue
			}
			is := &itemState{itemType: oi.Item.Type}
			switch oi.Item.Type {
			case "message":
				is.kind = llm.ContentText
			case "function_call":
				is.kind = llm.ContentToolCall
				is.callID = oi.Item.CallID
				is.name = oi.Item.Name
				if !stream.Send(llm.StreamEvent{
					Type:        llm.EventContentStart,
					Index:       oi.Index,
					ContentKind: llm.ContentToolCall,
					ToolCallID:  oi.Item.CallID,
					ToolName:    oi.Item.Name,
				}) {
					return
				}
			}
			items[oi.Index] = is

		case "response.content_part.added":
			var cp openaiSSEContentPart
			if json.Unmarshal([]byte(sse.Data), &cp) != nil {
				continue
			}
			if !stream.Send(llm.StreamEvent{
				Type:        llm.EventContentStart,
				Index:       cp.OutputIndex,
				ContentKind: llm.ContentText,
			}) {
				return
			}

		case "response.output_text.delta":
			var td openaiSSETextDelta
			if json.Unmarshal([]byte(sse.Data), &td) != nil {
				continue
			}
			if !stream.Send(llm.StreamEvent{
				Type:        llm.EventContentDelta,
				Index:       td.OutputIndex,
				ContentKind: llm.ContentText,
				Text:        td.Delta,
			}) {
				return
			}

		case "response.function_call_arguments.delta":
			var fd openaiSSEFuncArgsDelta
			if json.Unmarshal([]byte(sse.Data), &fd) != nil {
				continue
			}
			if !stream.Send(llm.StreamEvent{
				Type:        llm.EventContentDelta,
				Index:       fd.OutputIndex,
				ContentKind: llm.ContentToolCall,
				Text:        fd.Delta,
			}) {
				return
			}

		case "response.output_item.done":
			var oid openaiSSEOutputItemDone
			if json.Unmarshal([]byte(sse.Data), &oid) != nil {
				continue
			}
			// Accumulate into response.
			switch oid.Item.Type {
			case "message":
				for _, c := range oid.Item.Content {
					if c.Type == "output_text" || c.Type == "text" {
						resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
							Kind: llm.ContentText,
							Text: c.Text,
						})
					}
				}
			case "function_call":
				resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
					Kind: llm.ContentToolCall,
					ToolCall: &llm.ToolCallData{
						ID:        oid.Item.CallID,
						Name:      oid.Item.Name,
						Arguments: json.RawMessage(oid.Item.Arguments),
					},
				})
			}

			// Find the output index for this item.
			for idx, is := range items {
				if is != nil {
					if !stream.Send(llm.StreamEvent{
						Type:        llm.EventContentStop,
						Index:       idx,
						ContentKind: is.kind,
					}) {
						return
					}
					// Only stop the first matching item that matches type.
					// In practice each output_item.done fires per item.
					break
				}
			}

		case "response.completed":
			var rc openaiSSEResponse
			if json.Unmarshal([]byte(sse.Data), &rc) != nil {
				continue
			}

			var usage llm.Usage
			if rc.Usage != nil {
				usage = llm.Usage{
					InputTokens:     rc.Usage.InputTokens,
					OutputTokens:    rc.Usage.OutputTokens,
					TotalTokens:     rc.Usage.TotalTokens,
					ReasoningTokens: rc.Usage.OutputTokensDetails.ReasoningTokens,
				}
			}

			finish := llm.FinishStop
			if len(resp.ToolCalls()) > 0 {
				finish = llm.FinishToolUse
			}

			resp.FinishReason = finish
			resp.Usage = usage

			stream.Send(llm.StreamEvent{
				Type:         llm.EventUsage,
				Usage:        &usage,
				FinishReason: finish,
			})

			stream.Finish(resp, nil)
			return

		case "error":
			stream.Send(llm.StreamEvent{
				Type: llm.EventError,
				Err:  &llm.SDKError{Message: "openai stream error: " + sse.Data},
			})
			stream.Finish(nil, &llm.SDKError{Message: "openai stream error: " + sse.Data})
			return
		}
	}

	if err := scanner.Err(); err != nil {
		stream.Finish(nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "openai stream interrupted", Cause: err}})
		return
	}
	stream.Finish(nil, &llm.SDKError{Message: "openai stream ended without response.completed"})
}
