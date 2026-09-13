package llm

import "strings"

var bedrockModels = map[string]string{
	"opus":   "us.anthropic.claude-opus-4-6-v1",
	"sonnet": "us.anthropic.claude-sonnet-5-v1",
	"haiku":  "us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"fable":  "us.anthropic.claude-fable-5-1-v1",

	"claude-opus-5":              "us.anthropic.claude-opus-5-v1",
	"claude-opus-4-6":            "us.anthropic.claude-opus-4-6-v1",
	"claude-sonnet-5":            "us.anthropic.claude-sonnet-5-v1",
	"claude-sonnet-4-20250514":   "us.anthropic.claude-sonnet-4-20250514-v1:0",
	"claude-haiku-4-5-20251001":  "us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-fable-5-1":           "us.anthropic.claude-fable-5-1-v1",
}

func ResolveBedrockModel(hint string) string {
	if hint == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(hint))

	if strings.HasPrefix(lower, "us.anthropic.") || strings.HasPrefix(lower, "anthropic.") {
		return hint
	}

	if resolved, ok := bedrockModels[lower]; ok {
		return resolved
	}

	for key, val := range bedrockModels {
		if strings.Contains(lower, key) {
			return val
		}
	}

	return "us.anthropic." + lower + "-v1"
}

func ModelShortName(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		return "opus"
	case strings.Contains(lower, "sonnet"):
		return "sonnet"
	case strings.Contains(lower, "haiku"):
		return "haiku"
	case strings.Contains(lower, "fable"):
		return "fable"
	default:
		return model
	}
}

func modelMaxOutputTokens(model string) int {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		return 32768
	case strings.Contains(lower, "sonnet"):
		return 64000
	case strings.Contains(lower, "haiku"):
		return 8192
	case strings.Contains(lower, "fable"):
		return 64000
	default:
		return 16384
	}
}
