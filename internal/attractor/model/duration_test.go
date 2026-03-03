package model

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"45s", 45 * time.Second, false},
		{"15m", 15 * time.Minute, false},
		{"2h", 2 * time.Hour, false},
		{"250ms", 250 * time.Millisecond, false},
		{"1d", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"", 0, false},
		{"0d", 0, true},
		{"abc", 0, true},
		{"1.5d", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.expected {
				t.Fatalf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNodeDurationAttr(t *testing.T) {
	n := &Node{ID: "test", Attrs: map[string]string{
		"timeout":   "30s",
		"interval":  "2d",
		"bad_value": "xyz",
	}}

	if got := n.DurationAttr("timeout", time.Minute); got != 30*time.Second {
		t.Fatalf("expected 30s, got %v", got)
	}
	if got := n.DurationAttr("interval", time.Minute); got != 48*time.Hour {
		t.Fatalf("expected 48h, got %v", got)
	}
	if got := n.DurationAttr("bad_value", 5*time.Second); got != 5*time.Second {
		t.Fatalf("expected 5s default, got %v", got)
	}
	if got := n.DurationAttr("missing", 10*time.Second); got != 10*time.Second {
		t.Fatalf("expected 10s default, got %v", got)
	}
}

func TestGraphDurationAttr(t *testing.T) {
	g := NewGraph("test")
	g.Attrs["poll"] = "15m"

	if got := g.DurationAttr("poll", time.Hour); got != 15*time.Minute {
		t.Fatalf("expected 15m, got %v", got)
	}
	if got := g.DurationAttr("missing", time.Hour); got != time.Hour {
		t.Fatalf("expected 1h default, got %v", got)
	}
}
