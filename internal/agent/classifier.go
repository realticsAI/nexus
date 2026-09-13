package agent

import (
	"strings"

	"github.com/anurag/nexus/internal/jira"
)

func Classify(issue *jira.Issue) TicketType {
	typeLower := strings.ToLower(issue.Type)
	summaryLower := strings.ToLower(issue.Summary)
	descLower := strings.ToLower(issue.Description)

	if isPrep(summaryLower, descLower) {
		return Prep
	}

	switch {
	case typeLower == "bug" || strings.Contains(summaryLower, "fix") || strings.Contains(summaryLower, "bug"):
		return Bug
	case strings.Contains(summaryLower, "refactor") || strings.Contains(descLower, "refactor") ||
		strings.Contains(summaryLower, "cleanup") || strings.Contains(summaryLower, "tech debt"):
		return Refactor
	case typeLower == "task" || typeLower == "chore" || strings.Contains(summaryLower, "chore"):
		return Chore
	default:
		return Feature
	}
}

var prepSignals = []string{
	"feature preparation", "feature prep",
	"review and clarify", "review & clarify",
	"spike", "research", "investigation",
	"poc", "proof of concept",
	"discovery", "exploration",
	"requirements gathering", "requirement gathering",
	"design review", "architecture review",
	"tech review", "technical review",
	"feasibility", "assessment",
}

func isPrep(summary, desc string) bool {
	combined := summary + " " + desc
	for _, signal := range prepSignals {
		if strings.Contains(combined, signal) {
			return true
		}
	}
	return false
}
