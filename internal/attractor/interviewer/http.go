package interviewer

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// PendingQuestion is a question waiting for an HTTP answer.
type PendingQuestion struct {
	ID       string   `json:"id"`
	Question Question `json:"question"`
	answer   chan Answer
	ctx      context.Context
}

// HTTP is an Interviewer that bridges blocking engine calls with async HTTP requests.
// Ask blocks until SubmitAnswer is called from an HTTP handler or the context is canceled.
type HTTP struct {
	mu      sync.RWMutex
	pending map[string]*PendingQuestion
	ctx     context.Context
}

// NewHTTP creates a new HTTP interviewer. The context controls the lifetime of all
// pending questions — canceling it unblocks all waiting Ask calls.
func NewHTTP(ctx context.Context) *HTTP {
	return &HTTP{
		pending: make(map[string]*PendingQuestion),
		ctx:     ctx,
	}
}

// Ask blocks the calling goroutine until an answer is submitted via SubmitAnswer
// or the context is canceled.
func (h *HTTP) Ask(q Question) (Answer, error) {
	pq := &PendingQuestion{
		ID:       uuid.New().String(),
		Question: q,
		answer:   make(chan Answer, 1),
		ctx:      h.ctx,
	}

	h.mu.Lock()
	h.pending[pq.ID] = pq
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, pq.ID)
		h.mu.Unlock()
	}()

	select {
	case ans := <-pq.answer:
		return ans, nil
	case <-h.ctx.Done():
		return Answer{}, h.ctx.Err()
	}
}

// Pending returns a snapshot of all pending questions.
func (h *HTTP) Pending() []PendingQuestion {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]PendingQuestion, 0, len(h.pending))
	for _, pq := range h.pending {
		out = append(out, PendingQuestion{
			ID:       pq.ID,
			Question: pq.Question,
		})
	}
	return out
}

// SubmitAnswer sends an answer to a pending question, unblocking the engine goroutine.
// Returns an error if the question ID is not found.
func (h *HTTP) SubmitAnswer(qid string, ans Answer) error {
	h.mu.RLock()
	pq, ok := h.pending[qid]
	h.mu.RUnlock()
	if !ok {
		return fmt.Errorf("question %q not found", qid)
	}
	select {
	case pq.answer <- ans:
		return nil
	default:
		return fmt.Errorf("question %q already answered", qid)
	}
}
