package runtime

import (
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// RetryPolicy defines exponential backoff parameters.
type RetryPolicy struct {
	InitialDelayMs int
	BackoffFactor  float64
	MaxDelayMs     int
	Jitter         bool
}

// DelayForAttempt returns the delay for the given attempt number (1-based).
func (p RetryPolicy) DelayForAttempt(attempt int) time.Duration {
	if p.InitialDelayMs <= 0 {
		return 0
	}
	delay := float64(p.InitialDelayMs) * math.Pow(p.BackoffFactor, float64(attempt-1))
	if p.MaxDelayMs > 0 && delay > float64(p.MaxDelayMs) {
		delay = float64(p.MaxDelayMs)
	}
	if p.Jitter {
		// Jitter to 50-100% of computed delay
		delay = delay * (0.5 + 0.5*rand.Float64())
	}
	return time.Duration(delay) * time.Millisecond
}

var presets = map[string]RetryPolicy{
	"none":       {InitialDelayMs: 0, BackoffFactor: 1.0, MaxDelayMs: 0, Jitter: false},
	"standard":   {InitialDelayMs: 200, BackoffFactor: 2.0, MaxDelayMs: 60000, Jitter: true},
	"aggressive": {InitialDelayMs: 50, BackoffFactor: 1.5, MaxDelayMs: 5000, Jitter: true},
	"linear":     {InitialDelayMs: 1000, BackoffFactor: 1.0, MaxDelayMs: 1000, Jitter: false},
	"patient":    {InitialDelayMs: 1000, BackoffFactor: 3.0, MaxDelayMs: 120000, Jitter: true},
}

// PolicyByName returns a named retry policy preset. Falls back to "standard".
func PolicyByName(name string) RetryPolicy {
	if p, ok := presets[strings.ToLower(name)]; ok {
		return p
	}
	return presets["standard"]
}
