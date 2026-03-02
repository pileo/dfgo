package interviewer

import "fmt"

// Queue is an Interviewer that returns pre-filled answers in order.
// Useful for testing.
type Queue struct {
	Answers []Answer
	pos     int
}

// NewQueue creates a Queue from a list of answer strings.
func NewQueue(answers ...string) *Queue {
	q := &Queue{}
	for _, a := range answers {
		q.Answers = append(q.Answers, Answer{Text: a, Selected: -1})
	}
	return q
}

func (q *Queue) Ask(question Question) (Answer, error) {
	if q.pos >= len(q.Answers) {
		return Answer{}, fmt.Errorf("queue exhausted after %d answers", q.pos)
	}
	a := q.Answers[q.pos]
	q.pos++
	return a, nil
}

// Remaining returns the number of unused answers.
func (q *Queue) Remaining() int {
	return len(q.Answers) - q.pos
}
