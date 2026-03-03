package agent

import (
	"dfgo/internal/agent/message"
	"dfgo/internal/llm"
)

// Result holds the outcome of a completed agent session.
type Result struct {
	// Messages is the complete conversation history.
	Messages []message.Message

	// FinalText is the assistant's last text response.
	FinalText string

	// TotalUsage tracks cumulative token consumption.
	TotalUsage llm.Usage

	// Rounds is the number of LLM round-trips made.
	Rounds int

	// Aborted is true if the session was cancelled.
	Aborted bool

	// Error is non-nil if the session ended due to an unrecoverable error.
	Error error
}
