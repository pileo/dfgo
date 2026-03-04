package interviewer

import (
	"context"
	"testing"
	"time"
)

func TestHTTPAskAndAnswer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := NewHTTP(ctx)

	// Ask in background goroutine (blocks until answered).
	done := make(chan struct{})
	var gotAns Answer
	var gotErr error
	go func() {
		defer close(done)
		gotAns, gotErr = h.Ask(Question{
			Type:   Freeform,
			Prompt: "What is your name?",
		})
	}()

	// Wait for question to appear.
	var pending []PendingQuestion
	deadline := time.After(time.Second)
	for {
		pending = h.Pending()
		if len(pending) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending question")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	if len(pending) != 1 {
		t.Fatalf("expected 1 pending question, got %d", len(pending))
	}
	if pending[0].Question.Prompt != "What is your name?" {
		t.Errorf("unexpected prompt: %s", pending[0].Question.Prompt)
	}

	// Submit answer.
	err := h.SubmitAnswer(pending[0].ID, Answer{Text: "Alice"})
	if err != nil {
		t.Fatalf("SubmitAnswer error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Ask did not return after SubmitAnswer")
	}

	if gotErr != nil {
		t.Fatalf("Ask returned error: %v", gotErr)
	}
	if gotAns.Text != "Alice" {
		t.Errorf("expected answer 'Alice', got %q", gotAns.Text)
	}

	// After answer, no more pending questions.
	if len(h.Pending()) != 0 {
		t.Errorf("expected 0 pending questions after answer, got %d", len(h.Pending()))
	}
}

func TestHTTPCancelUnblocksAsk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := NewHTTP(ctx)

	done := make(chan error, 1)
	go func() {
		_, err := h.Ask(Question{Type: YesNo, Prompt: "Continue?"})
		done <- err
	}()

	// Wait for question to be pending.
	deadline := time.After(time.Second)
	for len(h.Pending()) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending question")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from canceled context, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("Ask did not return after context cancel")
	}
}

func TestHTTPSubmitAnswerNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := NewHTTP(ctx)
	err := h.SubmitAnswer("nonexistent", Answer{Text: "test"})
	if err == nil {
		t.Fatal("expected error for nonexistent question ID")
	}
}

func TestHTTPMultipleQuestions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := NewHTTP(ctx)

	// Ask two questions concurrently.
	done1 := make(chan Answer, 1)
	done2 := make(chan Answer, 1)

	go func() {
		ans, _ := h.Ask(Question{Type: Freeform, Prompt: "Q1"})
		done1 <- ans
	}()
	go func() {
		ans, _ := h.Ask(Question{Type: Freeform, Prompt: "Q2"})
		done2 <- ans
	}()

	// Wait for both to be pending.
	deadline := time.After(time.Second)
	for len(h.Pending()) < 2 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for 2 pending questions")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	pending := h.Pending()
	for _, pq := range pending {
		var text string
		if pq.Question.Prompt == "Q1" {
			text = "A1"
		} else {
			text = "A2"
		}
		if err := h.SubmitAnswer(pq.ID, Answer{Text: text}); err != nil {
			t.Fatalf("SubmitAnswer error: %v", err)
		}
	}

	select {
	case ans := <-done1:
		if ans.Text != "A1" {
			t.Errorf("expected A1, got %q", ans.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("Q1 did not return")
	}
	select {
	case ans := <-done2:
		if ans.Text != "A2" {
			t.Errorf("expected A2, got %q", ans.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("Q2 did not return")
	}
}
