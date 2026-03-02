package runtime

import (
	"strings"
	"testing"
)

func TestOutcomeIsSuccess(t *testing.T) {
	if !(Outcome{Status: StatusSuccess}).IsSuccess() {
		t.Fatal("SUCCESS should be success")
	}
	if !(Outcome{Status: StatusPartialSuccess}).IsSuccess() {
		t.Fatal("PARTIAL_SUCCESS should be success")
	}
	if (Outcome{Status: StatusFail}).IsSuccess() {
		t.Fatal("FAIL should not be success")
	}
	if (Outcome{Status: StatusRetry}).IsSuccess() {
		t.Fatal("RETRY should not be success")
	}
}

func TestOutcomeString(t *testing.T) {
	o := Outcome{Status: StatusSuccess, PreferredLabel: "yes"}
	s := o.String()
	if !strings.Contains(s, "SUCCESS") || !strings.Contains(s, "yes") {
		t.Fatalf("unexpected: %s", s)
	}
	o2 := FailOutcome("boom", FailureDeterministic)
	s2 := o2.String()
	if !strings.Contains(s2, "boom") {
		t.Fatalf("unexpected: %s", s2)
	}
}

func TestSuccessOutcome(t *testing.T) {
	o := SuccessOutcome()
	if o.Status != StatusSuccess {
		t.Fatal("expected SUCCESS")
	}
}

func TestSuccessOutcomeWithLabel(t *testing.T) {
	o := SuccessOutcomeWithLabel("approved")
	if o.Status != StatusSuccess || o.PreferredLabel != "approved" {
		t.Fatal("unexpected outcome")
	}
}

func TestFailOutcome(t *testing.T) {
	o := FailOutcome("timeout", FailureTransient)
	if o.Status != StatusFail || o.FailureReason != "timeout" || o.FailureClass != FailureTransient {
		t.Fatal("unexpected fail outcome")
	}
}

func TestRetryOutcome(t *testing.T) {
	o := RetryOutcome("temp error")
	if o.Status != StatusRetry || o.FailureClass != FailureTransient {
		t.Fatal("unexpected retry outcome")
	}
}
