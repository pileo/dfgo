// Package loop detects repetitive behavior in agent tool call sequences.
// It uses SHA-256 signature hashing over a sliding window to identify
// period-1, period-2, and period-3 loops.
package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Detector tracks tool call signatures and detects loops.
type Detector struct {
	window     int
	signatures []string
}

// NewDetector creates a loop detector with the given sliding window size.
func NewDetector(windowSize int) *Detector {
	if windowSize <= 0 {
		windowSize = 10
	}
	return &Detector{window: windowSize}
}

// Record adds a tool call signature and returns true if a loop is detected.
// The signature is computed from the tool name and arguments.
func (d *Detector) Record(toolName string, args string) bool {
	sig := hash(fmt.Sprintf("%s:%s", toolName, args))
	d.signatures = append(d.signatures, sig)

	// Trim to window size.
	if len(d.signatures) > d.window {
		d.signatures = d.signatures[len(d.signatures)-d.window:]
	}

	return d.detect()
}

// Reset clears the detector state.
func (d *Detector) Reset() {
	d.signatures = d.signatures[:0]
}

// detect checks for period-1, period-2, and period-3 loops.
func (d *Detector) detect() bool {
	n := len(d.signatures)

	// Period-1: last 3 identical signatures.
	if n >= 3 {
		if d.signatures[n-1] == d.signatures[n-2] && d.signatures[n-2] == d.signatures[n-3] {
			return true
		}
	}

	// Period-2: ABAB pattern in last 4.
	if n >= 4 {
		if d.signatures[n-1] == d.signatures[n-3] && d.signatures[n-2] == d.signatures[n-4] &&
			d.signatures[n-1] != d.signatures[n-2] {
			return true
		}
	}

	// Period-3: ABCABC pattern in last 6.
	if n >= 6 {
		if d.signatures[n-1] == d.signatures[n-4] &&
			d.signatures[n-2] == d.signatures[n-5] &&
			d.signatures[n-3] == d.signatures[n-6] {
			// Ensure not degenerate (all same).
			if !(d.signatures[n-1] == d.signatures[n-2] && d.signatures[n-2] == d.signatures[n-3]) {
				return true
			}
		}
	}

	return false
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
