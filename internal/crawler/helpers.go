package crawler

import "strings"

// extractStringArg pulls the first quoted string argument from a line of code.
// It handles both single and double quotes, and returns the string content
// (without quotes). Returns empty string if no quoted arg is found.
func extractStringArg(line string) string {
	// Try double quotes first
	if idx := strings.Index(line, "\""); idx >= 0 {
		rest := line[idx+1:]
		if end := strings.Index(rest, "\""); end >= 0 {
			return rest[:end]
		}
	}
	// Try single quotes
	if idx := strings.Index(line, "'"); idx >= 0 {
		rest := line[idx+1:]
		if end := strings.Index(rest, "'"); end >= 0 {
			return rest[:end]
		}
	}
	// Try backtick (template literals)
	if idx := strings.Index(line, "`"); idx >= 0 {
		rest := line[idx+1:]
		if end := strings.Index(rest, "`"); end >= 0 {
			return rest[:end]
		}
	}
	return ""
}
