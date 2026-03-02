package interviewer

import "strings"

// ParseAccelerator extracts the accelerator key from an option label.
// Patterns: "[Y] Yes", "Y) Yes", "(Y) Yes"
// Returns the key and the cleaned label, or empty string + original if no pattern found.
func ParseAccelerator(label string) (key string, clean string) {
	label = strings.TrimSpace(label)

	// Pattern: [X] Label
	if len(label) >= 4 && label[0] == '[' {
		idx := strings.Index(label, "]")
		if idx > 1 {
			key = strings.TrimSpace(label[1:idx])
			clean = strings.TrimSpace(label[idx+1:])
			return key, clean
		}
	}

	// Pattern: (X) Label
	if len(label) >= 4 && label[0] == '(' {
		idx := strings.Index(label, ")")
		if idx > 1 {
			key = strings.TrimSpace(label[1:idx])
			clean = strings.TrimSpace(label[idx+1:])
			return key, clean
		}
	}

	// Pattern: X) Label
	if len(label) >= 3 {
		idx := strings.Index(label, ")")
		if idx > 0 && idx <= 3 {
			key = strings.TrimSpace(label[:idx])
			clean = strings.TrimSpace(label[idx+1:])
			return key, clean
		}
	}

	return "", label
}

// MatchAccelerator checks if the input matches any option's accelerator key.
// Returns the matched option index, or -1.
func MatchAccelerator(input string, options []string) int {
	input = strings.TrimSpace(strings.ToUpper(input))
	for i, opt := range options {
		key, _ := ParseAccelerator(opt)
		if key != "" && strings.ToUpper(key) == input {
			return i
		}
	}
	return -1
}
