package security

import (
	"regexp"
	"strings"
)

type InjectionResult struct {
	Safe     bool
	Threats  []InjectionThreat
	Severity string
}

type InjectionThreat struct {
	Type     string
	Pattern  string
	Context  string
	Severity string
}

type injectionRule struct {
	typ      string
	severity string
	re       *regexp.Regexp
}

var injectionRules = []injectionRule{
	// Instruction overrides — high severity
	{"instruction_override", "high", regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above|earlier)\s+(instructions|prompts|rules|context)`)},
	{"instruction_override", "high", regexp.MustCompile(`(?i)disregard\s+(all\s+)?(previous|prior|above|earlier)`)},
	{"instruction_override", "high", regexp.MustCompile(`(?i)(you\s+are\s+now|act\s+as|pretend\s+(you\s+are|to\s+be)|roleplay\s+as)`)},
	{"instruction_override", "high", regexp.MustCompile(`(?i)(system\s+prompt|system\s+message|new\s+instructions)`)},
	{"instruction_override", "high", regexp.MustCompile(`(?i)(override|overwrite)\s+(your|the|all)\s+(instructions|rules|behavior|prompt)`)},

	// Shell injection — high severity
	{"shell_injection", "high", regexp.MustCompile(`(?i)rm\s+-rf\s+/`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)curl\s+[^\s]+\s*\|\s*(?:ba)?sh`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)wget\s+[^\s]+\s*(?:-O\s*-\s*)?\|\s*(?:ba)?sh`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)nc\s+-[elp]`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)chmod\s+777`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)mkfifo\s+/tmp/`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)/dev/tcp/`)},
	{"shell_injection", "high", regexp.MustCompile(`(?i)eval\s*\(\s*base64`)},

	// Behavior overrides — medium severity
	{"behavior_override", "medium", regexp.MustCompile(`(?i)do\s+not\s+follow\s+(the|your|any)\s+(rules|instructions|guidelines)`)},
	{"behavior_override", "medium", regexp.MustCompile(`(?i)(skip|bypass|disable|ignore)\s+(validation|safety|security|checks|review|guard)`)},
	{"behavior_override", "medium", regexp.MustCompile(`(?i)(skip|bypass|disable)\s+(the\s+)?(test|lint|ci|build)\s+(step|phase|check)`)},
	{"behavior_override", "medium", regexp.MustCompile(`(?i)push\s+(directly\s+)?to\s+(main|master)\s+without`)},
	{"behavior_override", "medium", regexp.MustCompile(`(?i)(force|auto)\s*-?\s*push\s+(to\s+)?(main|master|prod)`)},

	// Base64 payloads — low severity (may be legitimate, but flag it)
	{"base64_payload", "low", regexp.MustCompile(`[A-Za-z0-9+/]{100,}={0,2}`)},
}

var severityRank = map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3}

func ScanForInjection(text string) *InjectionResult {
	result := &InjectionResult{Safe: true, Severity: "none"}

	for _, rule := range injectionRules {
		matches := rule.re.FindAllStringIndex(text, -1)
		for _, loc := range matches {
			matched := text[loc[0]:loc[1]]
			ctx := extractContext(text, loc[0], loc[1], 50)

			result.Threats = append(result.Threats, InjectionThreat{
				Type:     rule.typ,
				Pattern:  matched,
				Context:  ctx,
				Severity: rule.severity,
			})
			result.Safe = false

			if severityRank[rule.severity] > severityRank[result.Severity] {
				result.Severity = rule.severity
			}
		}
	}

	return result
}

func extractContext(text string, start, end, window int) string {
	ctxStart := start - window
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := end + window
	if ctxEnd > len(text) {
		ctxEnd = len(text)
	}
	s := text[ctxStart:ctxEnd]
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}
