package interviewer

// AskFunc is the signature for a callback that answers questions.
type AskFunc func(Question) (Answer, error)

// Callback is an Interviewer that delegates to a user-supplied function.
type Callback struct {
	fn AskFunc
}

// NewCallback creates a Callback interviewer that delegates Ask to fn.
func NewCallback(fn AskFunc) *Callback {
	return &Callback{fn: fn}
}

func (c *Callback) Ask(q Question) (Answer, error) {
	return c.fn(q)
}
