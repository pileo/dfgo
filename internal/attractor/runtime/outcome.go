package runtime

import "fmt"

// StageStatus represents the result status of a pipeline stage execution.
type StageStatus string

const (
	StatusSuccess        StageStatus = "SUCCESS"
	StatusFail           StageStatus = "FAIL"
	StatusRetry          StageStatus = "RETRY"
	StatusPartialSuccess StageStatus = "PARTIAL_SUCCESS"
)

// FailureClass categorizes failures for retry/escalation decisions.
type FailureClass string

const (
	FailureTransient       FailureClass = "transient"
	FailureDeterministic   FailureClass = "deterministic"
	FailureCanceled        FailureClass = "canceled"
	FailureBudgetExhausted FailureClass = "budget_exhausted"
)

// Outcome is the result of executing a pipeline stage handler.
type Outcome struct {
	Status          StageStatus       `json:"status"`
	PreferredLabel  string            `json:"preferred_label,omitempty"`
	SuggestedNextIDs []string          `json:"suggested_next_ids,omitempty"`
	ContextUpdates  map[string]string `json:"context_updates,omitempty"`
	FailureReason   string            `json:"failure_reason,omitempty"`
	FailureClass    FailureClass      `json:"failure_class,omitempty"`
	Notes           string            `json:"notes,omitempty"`
}

// IsSuccess returns true if the status indicates success.
func (o Outcome) IsSuccess() bool {
	return o.Status == StatusSuccess || o.Status == StatusPartialSuccess
}

// String returns a human-readable representation.
func (o Outcome) String() string {
	s := fmt.Sprintf("Outcome(%s)", o.Status)
	if o.PreferredLabel != "" {
		s += fmt.Sprintf(" label=%q", o.PreferredLabel)
	}
	if o.FailureReason != "" {
		s += fmt.Sprintf(" reason=%q", o.FailureReason)
	}
	return s
}

// SuccessOutcome creates a simple success outcome.
func SuccessOutcome() Outcome {
	return Outcome{Status: StatusSuccess}
}

// SuccessOutcomeWithLabel creates a success outcome with a preferred label.
func SuccessOutcomeWithLabel(label string) Outcome {
	return Outcome{Status: StatusSuccess, PreferredLabel: label}
}

// FailOutcome creates a failure outcome.
func FailOutcome(reason string, class FailureClass) Outcome {
	return Outcome{
		Status:        StatusFail,
		FailureReason: reason,
		FailureClass:  class,
	}
}

// RetryOutcome creates a retry outcome.
func RetryOutcome(reason string) Outcome {
	return Outcome{
		Status:        StatusRetry,
		FailureReason: reason,
		FailureClass:  FailureTransient,
	}
}
