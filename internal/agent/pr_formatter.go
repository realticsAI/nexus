package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/anurag/nexus/internal/jira"
)

func FormatPRTitle(ticketKey string, issue *jira.Issue) string {
	return fmt.Sprintf("%s: %s", ticketKey, issue.Summary)
}

func FormatPRBody(ticketKey string, issue *jira.Issue, ticketType TicketType, investigation *InvestigationContext, analysis *AnalysisResult, plan *Plan, validation *ValidationResult, filesChanged []string, tdConfluenceURL string) string {
	var b strings.Builder

	// ── Summary ──────────────────────────────────────────────────────
	b.WriteString("## Summary\n\n")
	if plan != nil && plan.Summary != "" {
		b.WriteString(fmt.Sprintf("- %s\n", plan.Summary))
	}

	if analysis != nil {
		musts := 0
		for _, r := range analysis.Requirements {
			if r.Priority == "must" {
				musts++
			}
		}
		if musts > 0 {
			b.WriteString(fmt.Sprintf("- Implements %d must-have requirements", musts))
			if len(analysis.EdgeCases) > 0 {
				b.WriteString(fmt.Sprintf(", handles %d edge cases", len(analysis.EdgeCases)))
			}
			b.WriteString("\n")
		}
	}

	if validation != nil {
		for _, d := range validation.Dimensions {
			if d.Name == "error_handling" && d.Verdict == "PASS" {
				b.WriteString("- Graceful degradation: downstream failures handled with fallbacks\n")
				break
			}
		}
	}

	// ── Changes (per-file breakdown) ─────────────────────────────────
	if plan != nil && len(plan.Steps) > 0 {
		b.WriteString("\n## Changes\n")

		grouped := groupStepsByFile(plan.Steps)
		for _, fg := range grouped {
			b.WriteString(fmt.Sprintf("\n### %s", filepath.Base(fg.File)))
			if fg.Action == "create" {
				b.WriteString(" (NEW)")
			}
			b.WriteString("\n")
			for _, desc := range fg.Descriptions {
				b.WriteString(fmt.Sprintf("- %s\n", desc))
			}
			if len(fg.Covers) > 0 {
				b.WriteString(fmt.Sprintf("- Covers: %s\n", strings.Join(fg.Covers, ", ")))
			}
		}
	}

	// ── Technical Design ─────────────────────────────────────────────
	b.WriteString("\n## Technical Design\n\n")
	if tdConfluenceURL != "" {
		b.WriteString(fmt.Sprintf("- **TD:** [Confluence — %s](%s)\n", ticketKey, tdConfluenceURL))
	}
	b.WriteString(fmt.Sprintf("- **Ticket:** [%s](https://jira.example.com/browse/%s)\n", ticketKey, ticketKey))

	if investigation != nil {
		for _, fd := range investigation.FigmaDesigns {
			b.WriteString(fmt.Sprintf("- **Figma:** [%s](%s)\n", fd.FileName, fd.URL))
		}
		for _, li := range investigation.LinkedIssues {
			label := "Linked"
			if li.Type == "Spike" || li.Type == "Sub-task" {
				label = li.Type
			}
			b.WriteString(fmt.Sprintf("- **%s:** [%s](https://jira.example.com/browse/%s) — %s\n", label, li.Key, li.Key, li.Summary))
		}
	}

	// ── Test plan ────────────────────────────────────────────────────
	b.WriteString("\n## Test plan\n\n")
	b.WriteString("- [ ] Verify build compiles\n")
	b.WriteString("- [ ] Unit tests pass\n")

	if analysis != nil {
		for _, r := range analysis.Requirements {
			if r.Priority == "must" {
				b.WriteString(fmt.Sprintf("- [ ] %s: %s\n", r.ID, r.Description))
			}
		}
		for i, ec := range analysis.EdgeCases {
			b.WriteString(fmt.Sprintf("- [ ] EC-%d: %s → %s\n", i+1, ec.Trigger, ec.Expected))
		}
	}

	if validation != nil {
		for _, d := range validation.Dimensions {
			if d.Name == "error_handling" {
				b.WriteString(fmt.Sprintf("- [ ] Error handling: %s\n", d.Detail))
			}
			if d.Name == "security" {
				b.WriteString(fmt.Sprintf("- [ ] Security: %s\n", d.Detail))
			}
		}
	}

	b.WriteString("- [ ] Deploy to TST and verify via Splunk + Postman\n")
	b.WriteString("- [ ] QA sign-off\n")

	// ── Dependencies ─────────────────────────────────────────────────
	if investigation != nil && (len(investigation.ExternalAPIs) > 0 || len(investigation.Dependencies) > 0) {
		b.WriteString("\n## Dependencies\n\n")
		for _, api := range investigation.ExternalAPIs {
			label := "External"
			if api.Internal {
				label = "Internal"
			}
			b.WriteString(fmt.Sprintf("- %s: %s", label, api.FacadeClass))
			if len(api.URLs) > 0 {
				b.WriteString(fmt.Sprintf(" (`%s`)", api.URLs[0]))
			}
			b.WriteString("\n")
		}
		for _, dep := range investigation.Dependencies {
			b.WriteString(fmt.Sprintf("- %s\n", dep))
		}
	}

	return b.String()
}

type fileGroup struct {
	File         string
	Action       string
	Descriptions []string
	Covers       []string
}

func groupStepsByFile(steps []PlanStep) []fileGroup {
	seen := make(map[string]int)
	var groups []fileGroup

	for _, step := range steps {
		key := step.File
		if idx, ok := seen[key]; ok {
			desc := stepDescription(step)
			if desc != "" {
				groups[idx].Descriptions = append(groups[idx].Descriptions, desc)
			}
			for _, c := range step.Covers {
				groups[idx].Covers = append(groups[idx].Covers, c)
			}
		} else {
			seen[key] = len(groups)
			desc := stepDescription(step)
			var descs []string
			if desc != "" {
				descs = []string{desc}
			}
			groups = append(groups, fileGroup{
				File:         step.File,
				Action:       step.Action,
				Descriptions: descs,
				Covers:       step.Covers,
			})
		}
	}

	for i := range groups {
		groups[i].Covers = dedupStrings(groups[i].Covers)
	}

	return groups
}

func stepDescription(step PlanStep) string {
	content := strings.TrimSpace(step.Content)
	if content == "" {
		return ""
	}
	lines := strings.SplitN(content, "\n", 2)
	first := strings.TrimSpace(lines[0])
	if len(first) > 200 {
		first = first[:197] + "..."
	}
	return first
}

func dedupStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
