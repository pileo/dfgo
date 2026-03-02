package interviewer

// Interaction records a question and its answer.
type Interaction struct {
	Question Question
	Answer   Answer
}

// Recording wraps another Interviewer and records all interactions.
type Recording struct {
	Inner        Interviewer
	Interactions []Interaction
}

// NewRecording creates a Recording that wraps inner.
func NewRecording(inner Interviewer) *Recording {
	return &Recording{Inner: inner}
}

func (r *Recording) Ask(q Question) (Answer, error) {
	a, err := r.Inner.Ask(q)
	if err != nil {
		return a, err
	}
	r.Interactions = append(r.Interactions, Interaction{Question: q, Answer: a})
	return a, nil
}
