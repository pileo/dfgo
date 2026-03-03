package runtime

import (
	"testing"
	"time"
)

func TestDelayForAttemptStandard(t *testing.T) {
	p := PolicyByName("standard")
	d1 := p.DelayForAttempt(1)
	if d1 < 100*time.Millisecond || d1 > 200*time.Millisecond {
		t.Errorf("attempt 1: expected 100-200ms, got %v", d1)
	}
	d2 := p.DelayForAttempt(2)
	if d2 < 200*time.Millisecond || d2 > 400*time.Millisecond {
		t.Errorf("attempt 2: expected 200-400ms, got %v", d2)
	}
}

func TestDelayForAttemptNone(t *testing.T) {
	p := PolicyByName("none")
	if d := p.DelayForAttempt(1); d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
	if d := p.DelayForAttempt(5); d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestDelayForAttemptCapped(t *testing.T) {
	p := PolicyByName("aggressive")
	// After many attempts, delay should be capped at MaxDelayMs (5000ms)
	d := p.DelayForAttempt(100)
	if d > 5*time.Second {
		t.Errorf("expected capped at 5s, got %v", d)
	}
}

func TestDelayForAttemptLinear(t *testing.T) {
	p := PolicyByName("linear")
	// Linear: factor=1.0, no jitter, so every attempt is exactly 1000ms
	d1 := p.DelayForAttempt(1)
	if d1 != 1000*time.Millisecond {
		t.Errorf("attempt 1: expected 1000ms, got %v", d1)
	}
	d5 := p.DelayForAttempt(5)
	if d5 != 1000*time.Millisecond {
		t.Errorf("attempt 5: expected 1000ms, got %v", d5)
	}
}

func TestPolicyByNameFallback(t *testing.T) {
	p := PolicyByName("unknown")
	std := PolicyByName("standard")
	if p.InitialDelayMs != std.InitialDelayMs {
		t.Fatal("unknown name should fall back to standard")
	}
}

func TestPolicyByNameAllPresets(t *testing.T) {
	names := []string{"none", "standard", "aggressive", "linear", "patient"}
	for _, name := range names {
		p := PolicyByName(name)
		if name != "none" && p.InitialDelayMs == 0 {
			t.Errorf("preset %q should have non-zero initial delay", name)
		}
	}
}
