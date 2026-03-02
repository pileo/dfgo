// Package interviewer defines the interface for human interaction during pipeline execution.
package interviewer

// QuestionType identifies the kind of question.
type QuestionType int

const (
	YesNo          QuestionType = iota
	MultipleChoice              // choices are in Options
	Freeform
	Confirmation
)

// Question represents a prompt to a human.
type Question struct {
	Type    QuestionType
	Prompt  string
	Options []string // for MultipleChoice; may contain accelerator keys like "[Y] Yes"
	Default string   // default answer if any
}

// Answer is the human's response.
type Answer struct {
	Text     string
	Selected int // index for MultipleChoice, -1 otherwise
}

// Interviewer is the interface for asking humans questions.
type Interviewer interface {
	Ask(q Question) (Answer, error)
}
