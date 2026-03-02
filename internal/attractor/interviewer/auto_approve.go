package interviewer

// AutoApprove always answers affirmatively.
type AutoApprove struct{}

func (a *AutoApprove) Ask(q Question) (Answer, error) {
	switch q.Type {
	case YesNo, Confirmation:
		return Answer{Text: "yes"}, nil
	case MultipleChoice:
		if len(q.Options) > 0 {
			return Answer{Text: q.Options[0], Selected: 0}, nil
		}
		return Answer{Text: "yes"}, nil
	case Freeform:
		if q.Default != "" {
			return Answer{Text: q.Default}, nil
		}
		return Answer{Text: "yes"}, nil
	}
	return Answer{Text: "yes"}, nil
}
