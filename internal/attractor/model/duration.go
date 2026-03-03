package model

import (
	"strings"
	"time"
)

// ParseDuration parses a duration string like "900s", "15m", "2h", "250ms", "1d".
// Returns an error if the string cannot be parsed.
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	// Handle "d" suffix (days) which time.ParseDuration doesn't support.
	if strings.HasSuffix(s, "d") {
		trimmed := strings.TrimSuffix(s, "d")
		var days int
		for _, c := range trimmed {
			if c < '0' || c > '9' {
				return 0, &time.ParseError{Layout: "duration", Value: s, Message: "invalid day duration"}
			}
			days = days*10 + int(c-'0')
		}
		if days > 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
		return 0, &time.ParseError{Layout: "duration", Value: s, Message: "zero day duration"}
	}

	return time.ParseDuration(s)
}

// DurationAttr returns the duration value of the named attribute, or the default.
// Supports "900s", "15m", "2h", "250ms", "1d".
func (n *Node) DurationAttr(key string, def time.Duration) time.Duration {
	v, ok := n.Attrs[key]
	if !ok {
		return def
	}
	d, err := ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// DurationAttr returns the duration value of the named graph attribute, or the default.
func (g *Graph) DurationAttr(key string, def time.Duration) time.Duration {
	v, ok := g.Attrs[key]
	if !ok {
		return def
	}
	d, err := ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
