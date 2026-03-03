package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"dfgo/internal/llm"

	"github.com/google/uuid"
)

// CompleteStream sends a streaming request to the Gemini API.
func (g *Gemini) CompleteStream(ctx context.Context, req llm.Request) (*llm.Stream, error) {
	body, err := g.buildRequest(req)
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to build gemini stream request", Cause: err}
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s", g.BaseURL, req.Model, g.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, &llm.SDKError{Message: "failed to create http request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := g.Client.Do(httpReq)
	if err != nil {
		return nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "gemini stream request failed", Cause: err}}
	}

	if httpResp.StatusCode != http.StatusOK {
		httpResp.Body.Close()
		return nil, llm.NewProviderError("gemini", httpResp.StatusCode,
			fmt.Sprintf("gemini API error (stream): status %d", httpResp.StatusCode), nil)
	}

	stream := llm.NewStream(ctx, httpResp.Body, 64)
	go g.readStream(stream)
	return stream, nil
}

// readStream reads SSE events from the Gemini API. Each SSE data event is a
// complete geminiResponse chunk. We diff successive chunks to emit content
// start/delta/stop events.
func (g *Gemini) readStream(stream *llm.Stream) {
	scanner := llm.NewSSEScanner(stream.Body())

	resp := &llm.Response{
		Provider: "gemini",
		Message:  llm.Message{Role: llm.RoleAssistant},
	}
	var usage llm.Usage

	metaSent := false
	prevPartCount := 0

	// Track which parts have been started for content.start/stop events.
	type partState struct {
		kind   llm.ContentKind
		toolID string
	}
	activeParts := make(map[int]*partState)

	for scanner.Next() {
		sse := scanner.Event()

		// Gemini streams each chunk as a JSON object in the data field.
		var chunk geminiResponse
		if json.Unmarshal([]byte(sse.Data), &chunk) != nil {
			continue
		}

		// Emit response.meta on first chunk.
		if !metaSent {
			metaSent = true
			if !stream.Send(llm.StreamEvent{
				Type:  llm.EventResponseMeta,
				Model: "", // Gemini doesn't echo the model in responses.
			}) {
				return
			}
		}

		// Update usage from each chunk (last one wins).
		usage = llm.Usage{
			InputTokens:  chunk.UsageMetadata.PromptTokenCount,
			OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  chunk.UsageMetadata.TotalTokenCount,
		}

		if len(chunk.Candidates) == 0 {
			continue
		}
		cand := chunk.Candidates[0]

		// Process parts in the candidate.
		for i, part := range cand.Content.Parts {
			if i >= prevPartCount {
				// New part — emit content.start.
				ps := &partState{}
				if part.Text != "" {
					ps.kind = llm.ContentText
					if !stream.Send(llm.StreamEvent{
						Type:        llm.EventContentStart,
						Index:       i,
						ContentKind: llm.ContentText,
					}) {
						return
					}
				} else if part.FunctionCall != nil {
					ps.kind = llm.ContentToolCall
					ps.toolID = uuid.New().String()
					if !stream.Send(llm.StreamEvent{
						Type:        llm.EventContentStart,
						Index:       i,
						ContentKind: llm.ContentToolCall,
						ToolCallID:  ps.toolID,
						ToolName:    part.FunctionCall.Name,
					}) {
						return
					}
				}
				activeParts[i] = ps
			}

			// Emit delta.
			ps := activeParts[i]
			if ps == nil {
				continue
			}
			if part.Text != "" {
				if !stream.Send(llm.StreamEvent{
					Type:        llm.EventContentDelta,
					Index:       i,
					ContentKind: llm.ContentText,
					Text:        part.Text,
				}) {
					return
				}
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				if !stream.Send(llm.StreamEvent{
					Type:        llm.EventContentDelta,
					Index:       i,
					ContentKind: llm.ContentToolCall,
					Text:        string(args),
				}) {
					return
				}
			}
		}
		prevPartCount = len(cand.Content.Parts)

		// If this chunk has a finish reason, it's the last one.
		if cand.FinishReason != "" {
			// Close all active parts.
			for idx, ps := range activeParts {
				stream.Send(llm.StreamEvent{
					Type:        llm.EventContentStop,
					Index:       idx,
					ContentKind: ps.kind,
				})
			}

			// Accumulate final response content.
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
						Kind: llm.ContentText,
						Text: part.Text,
					})
				}
				if part.FunctionCall != nil {
					args, _ := json.Marshal(part.FunctionCall.Args)
					ps := activeParts[0] // Use first part's ID as fallback.
					toolID := ""
					for idx, p := range activeParts {
						if p.kind == llm.ContentToolCall {
							toolID = p.toolID
							_ = idx
							break
						}
					}
					if toolID == "" {
						toolID = uuid.New().String()
					}
					resp.Message.Content = append(resp.Message.Content, llm.ContentPart{
						Kind: llm.ContentToolCall,
						ToolCall: &llm.ToolCallData{
							ID:        toolID,
							Name:      part.FunctionCall.Name,
							Arguments: args,
						},
					})
					_ = ps
				}
			}

			finish := llm.FinishStop
			switch cand.FinishReason {
			case "STOP":
				if len(resp.ToolCalls()) > 0 {
					finish = llm.FinishToolUse
				}
			case "MAX_TOKENS":
				finish = llm.FinishLength
			case "SAFETY":
				finish = llm.FinishContentFilter
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
		}
	}

	if err := scanner.Err(); err != nil {
		stream.Finish(nil, &llm.NetworkError{SDKError: llm.SDKError{Message: "gemini stream interrupted", Cause: err}})
		return
	}
	stream.Finish(nil, &llm.SDKError{Message: "gemini stream ended without finish reason"})
}
