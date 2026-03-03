package llm

import (
	"io"
	"strings"
	"testing"
)

func TestSSEScannerBasic(t *testing.T) {
	input := "event: message_start\ndata: {\"type\":\"message\"}\n\nevent: content\ndata: hello\n\n"
	s := NewSSEScanner(strings.NewReader(input))

	if !s.Next() {
		t.Fatal("expected first event")
	}
	e := s.Event()
	if e.Event != "message_start" {
		t.Errorf("event = %q, want %q", e.Event, "message_start")
	}
	if e.Data != `{"type":"message"}` {
		t.Errorf("data = %q", e.Data)
	}

	if !s.Next() {
		t.Fatal("expected second event")
	}
	e = s.Event()
	if e.Event != "content" {
		t.Errorf("event = %q, want %q", e.Event, "content")
	}
	if e.Data != "hello" {
		t.Errorf("data = %q, want %q", e.Data, "hello")
	}

	if s.Next() {
		t.Error("expected end of stream")
	}
	if s.Err() != io.EOF {
		t.Errorf("err = %v, want io.EOF", s.Err())
	}
}

func TestSSEScannerMultiLineData(t *testing.T) {
	input := "event: multi\ndata: line1\ndata: line2\ndata: line3\n\n"
	s := NewSSEScanner(strings.NewReader(input))

	if !s.Next() {
		t.Fatal("expected event")
	}
	e := s.Event()
	if e.Data != "line1\nline2\nline3" {
		t.Errorf("data = %q, want multi-line", e.Data)
	}
}

func TestSSEScannerComments(t *testing.T) {
	input := ": this is a comment\nevent: test\ndata: value\n\n"
	s := NewSSEScanner(strings.NewReader(input))

	if !s.Next() {
		t.Fatal("expected event")
	}
	if s.Event().Event != "test" {
		t.Errorf("event = %q", s.Event().Event)
	}
}

func TestSSEScannerEmptyLines(t *testing.T) {
	// Multiple empty lines between events should not produce empty events.
	input := "\n\nevent: first\ndata: one\n\n\n\nevent: second\ndata: two\n\n"
	s := NewSSEScanner(strings.NewReader(input))

	if !s.Next() {
		t.Fatal("expected first event")
	}
	if s.Event().Event != "first" {
		t.Errorf("event = %q", s.Event().Event)
	}
	if !s.Next() {
		t.Fatal("expected second event")
	}
	if s.Event().Event != "second" {
		t.Errorf("event = %q", s.Event().Event)
	}
}

func TestSSEScannerID(t *testing.T) {
	input := "id: 42\nevent: ping\ndata: pong\n\n"
	s := NewSSEScanner(strings.NewReader(input))

	if !s.Next() {
		t.Fatal("expected event")
	}
	e := s.Event()
	if e.ID != "42" {
		t.Errorf("id = %q, want %q", e.ID, "42")
	}
}

func TestSSEScannerEOFMidEvent(t *testing.T) {
	// Data without trailing empty line — should still emit on EOF.
	input := "event: partial\ndata: value"
	s := NewSSEScanner(strings.NewReader(input))

	if !s.Next() {
		t.Fatal("expected event from EOF")
	}
	if s.Event().Data != "value" {
		t.Errorf("data = %q", s.Event().Data)
	}
	if s.Err() != io.EOF {
		t.Errorf("err = %v, want io.EOF", s.Err())
	}

	if s.Next() {
		t.Error("expected no more events")
	}
}

func TestSSEScannerNoData(t *testing.T) {
	// Empty stream.
	s := NewSSEScanner(strings.NewReader(""))
	if s.Next() {
		t.Error("expected no events from empty input")
	}
	if s.Err() != io.EOF {
		t.Errorf("err = %v, want io.EOF", s.Err())
	}
}

func TestSSEScannerDataWithLeadingSpace(t *testing.T) {
	// Per SSE spec, one leading space after colon is stripped.
	input := "data: hello world\n\n"
	s := NewSSEScanner(strings.NewReader(input))
	if !s.Next() {
		t.Fatal("expected event")
	}
	if s.Event().Data != "hello world" {
		t.Errorf("data = %q, want %q", s.Event().Data, "hello world")
	}
}

func TestSSEScannerDataNoSpace(t *testing.T) {
	// No space after colon.
	input := "data:hello\n\n"
	s := NewSSEScanner(strings.NewReader(input))
	if !s.Next() {
		t.Fatal("expected event")
	}
	if s.Event().Data != "hello" {
		t.Errorf("data = %q, want %q", s.Event().Data, "hello")
	}
}

func TestSSEScannerCRLF(t *testing.T) {
	input := "event: test\r\ndata: value\r\n\r\n"
	s := NewSSEScanner(strings.NewReader(input))
	if !s.Next() {
		t.Fatal("expected event")
	}
	e := s.Event()
	if e.Event != "test" || e.Data != "value" {
		t.Errorf("event=%q data=%q", e.Event, e.Data)
	}
}
