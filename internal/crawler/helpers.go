package crawler

import "strings"

// containsAny returns true if s contains any of the given substrings.
// Both s and each needle are expected to already be lowercased by the caller.
func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
