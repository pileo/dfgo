package llm

import (
	"bufio"
	"io"
	"strings"
)

// SSEEvent represents a single Server-Sent Event.
type SSEEvent struct {
	Event string // event type (from "event:" line)
	Data  string // data payload (from "data:" line(s), joined with \n)
	ID    string // optional event ID
}

// SSEScanner reads SSE events from a stream following the W3C spec.
// Use Next() to advance, Event() to read the current event, Err() for errors.
type SSEScanner struct {
	reader *bufio.Reader
	cur    SSEEvent
	err    error
}

// NewSSEScanner creates a scanner that reads SSE events from r.
func NewSSEScanner(r io.Reader) *SSEScanner {
	return &SSEScanner{reader: bufio.NewReader(r)}
}

// Next advances to the next SSE event. Returns false when the stream ends or
// an error occurs (check Err()).
func (s *SSEScanner) Next() bool {
	var event, id string
	var dataLines []string
	hasField := false

	for {
		line, readErr := s.reader.ReadString('\n')
		// Trim trailing \r\n or \n.
		line = strings.TrimRight(line, "\r\n")

		// Process the line content even if we got an error (EOF returns
		// the partial line together with the error).
		if line == "" && readErr == nil {
			// Empty line = event boundary.
			if hasField {
				s.cur = SSEEvent{
					Event: event,
					Data:  strings.Join(dataLines, "\n"),
					ID:    id,
				}
				return true
			}
			// Consecutive empty lines — keep scanning.
			continue
		}

		if line != "" {
			// Comment lines (starting with ':') are ignored.
			if !strings.HasPrefix(line, ":") {
				// Parse field: value.
				field, value, _ := strings.Cut(line, ":")
				// Per spec, strip one leading space from value if present.
				value = strings.TrimPrefix(value, " ")

				switch field {
				case "event":
					event = value
					hasField = true
				case "data":
					dataLines = append(dataLines, value)
					hasField = true
				case "id":
					id = value
					hasField = true
				}
				// Unknown fields are ignored per spec.
			}
		}

		if readErr != nil {
			// If we accumulated data before EOF, emit the event.
			if readErr == io.EOF && hasField {
				s.cur = SSEEvent{
					Event: event,
					Data:  strings.Join(dataLines, "\n"),
					ID:    id,
				}
				s.err = io.EOF
				return true
			}
			if readErr == io.EOF {
				s.err = io.EOF
				return false
			}
			s.err = readErr
			return false
		}
	}
}

// Event returns the current SSE event. Only valid after Next() returns true.
func (s *SSEScanner) Event() SSEEvent {
	return s.cur
}

// Err returns the error that caused Next() to return false.
// Returns io.EOF for normal stream termination.
func (s *SSEScanner) Err() error {
	return s.err
}
