package security

import (
	"fmt"
	"regexp"
	"strings"
)

type ScanResult struct {
	Found    []SecretMatch
	Redacted string
}

type SecretMatch struct {
	Type   string
	Line   int
	Offset int
	Length int
}

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var patterns = []secretPattern{
	{"aws_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"aws_secret", regexp.MustCompile(`(?i)(?:aws_secret_access_key|aws_secret|secret_key)\s*[=:]\s*["']?([A-Za-z0-9/+=]{40})["']?`)},
	{"github_token", regexp.MustCompile(`gh[pors]_[A-Za-z0-9]{36,}`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)},
	{"private_key", regexp.MustCompile(`-----BEGIN[A-Z ]*PRIVATE KEY-----`)},
	{"connection_string", regexp.MustCompile(`(?i)(?:jdbc:|mongodb\+srv://|mongodb://|redis://|amqp://|mysql://|postgres(?:ql)?://)[^\s"'` + "`" + `]+`)},
	{"generic_secret", regexp.MustCompile(`(?i)(?:api_key|apikey|api-key|token|secret|password|passwd|credential)\s*[=:]\s*["']([A-Za-z0-9/+=_-]{16,})["']`)},
}

func ScanForSecrets(text string) *ScanResult {
	result := &ScanResult{}

	lines := strings.Split(text, "\n")
	lineOffsets := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		lineOffsets[i] = offset
		offset += len(line) + 1
	}

	findLine := func(pos int) int {
		for i := len(lineOffsets) - 1; i >= 0; i-- {
			if pos >= lineOffsets[i] {
				return i + 1
			}
		}
		return 1
	}

	type matchSpan struct {
		start, end int
		label      string
	}
	var spans []matchSpan

	for _, p := range patterns {
		for _, loc := range p.re.FindAllStringIndex(text, -1) {
			result.Found = append(result.Found, SecretMatch{
				Type:   p.name,
				Line:   findLine(loc[0]),
				Offset: loc[0],
				Length: loc[1] - loc[0],
			})
			spans = append(spans, matchSpan{loc[0], loc[1], p.name})
		}
	}

	if len(spans) == 0 {
		result.Redacted = text
		return result
	}

	// Sort spans by start offset (simple insertion sort — small N)
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}

	// Merge overlapping spans
	merged := []matchSpan{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end {
			if s.end > last.end {
				last.end = s.end
			}
		} else {
			merged = append(merged, s)
		}
	}

	var b strings.Builder
	prev := 0
	for _, s := range merged {
		b.WriteString(text[prev:s.start])
		b.WriteString(fmt.Sprintf("[REDACTED:%s]", s.label))
		prev = s.end
	}
	b.WriteString(text[prev:])
	result.Redacted = b.String()

	return result
}

func RedactSecrets(text string) string {
	return ScanForSecrets(text).Redacted
}
