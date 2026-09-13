package agent

import (
	"fmt"
	"strings"

	"github.com/anurag/nexus/internal/jira"
)

func FormatHandoff(result *WorkResult, issueDescription string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s — %s\n\n", result.TicketKey, string(result.TicketType)))

	// Target repos — the agent needs to know which repos to work in
	if result.Investigation != nil && len(result.Investigation.Services) > 0 {
		sb.WriteString("## Target\n\n")
		if len(result.Investigation.ServicePaths) > 1 {
			sb.WriteString("**Multi-repo ticket** — changes needed in multiple repositories:\n\n")
			for _, svcKey := range result.Investigation.Services {
				path := result.Investigation.ServicePaths[svcKey]
				if path == "" {
					continue
				}
				primary := ""
				if svcKey == result.Investigation.Services[0] {
					primary = " **(primary)**"
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s → `%s`\n", svcKey, primary, path))
			}
			sb.WriteString(fmt.Sprintf("\nBranch: `%s`\n", branchName(result)))
		} else {
			primary := result.Investigation.Services[0]
			sb.WriteString(fmt.Sprintf("Service: `%s`\n", primary))
			if result.Investigation.RepoPath != "" {
				sb.WriteString(fmt.Sprintf("Path: `%s`\n", result.Investigation.RepoPath))
				sb.WriteString(fmt.Sprintf("Branch: `%s`\n", branchName(result)))
			}
		}
		sb.WriteString("\n")
	}

	// Requirements — compact, actionable
	if result.Analysis != nil && len(result.Analysis.Requirements) > 0 {
		sb.WriteString("## Requirements\n\n")
		for _, r := range result.Analysis.Requirements {
			if r.Priority == "existing" {
				continue
			}
			prio := ""
			if r.Priority == "must" {
				prio = " [MUST]"
			}
			sb.WriteString(fmt.Sprintf("- **%s**%s: %s → %s\n", r.ID, prio, r.Description, r.CodeImpact))
		}
		sb.WriteString("\n")
	}

	// Edge cases
	if result.Analysis != nil && len(result.Analysis.EdgeCases) > 0 {
		sb.WriteString("## Edge Cases\n\n")
		for i, ec := range result.Analysis.EdgeCases {
			sb.WriteString(fmt.Sprintf("- EC-%d: %s → %s\n", i+1, ec.Trigger, ec.Expected))
		}
		sb.WriteString("\n")
	}

	// Plan steps — full content, no truncation
	if result.Plan != nil && len(result.Plan.Steps) > 0 {
		sb.WriteString(fmt.Sprintf("## Plan (%d steps)\n\n", len(result.Plan.Steps)))
		sb.WriteString(fmt.Sprintf("%s\n\n", result.Plan.Summary))

		// Collect files to read upfront
		readFiles := collectFilesToRead(result)
		if len(readFiles) > 0 {
			sb.WriteString("**Read these files first** (before making any changes):\n")
			for _, f := range readFiles {
				sb.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			sb.WriteString("\n")
		}

		// Group steps by service for multi-repo clarity
		isMultiRepo := hasMultipleServices(result.Plan.Steps)
		lastService := ""
		for i, step := range result.Plan.Steps {
			if isMultiRepo && step.Service != "" && step.Service != lastService {
				path := ""
				if result.Investigation != nil {
					path = result.Investigation.ServicePaths[step.Service]
				}
				if path != "" {
					sb.WriteString(fmt.Sprintf("#### Repo: `%s` (`%s`)\n\n", step.Service, path))
				} else {
					sb.WriteString(fmt.Sprintf("#### Repo: `%s`\n\n", step.Service))
				}
				lastService = step.Service
			}
			covers := ""
			if len(step.Covers) > 0 {
				covers = fmt.Sprintf(" [%s]", strings.Join(step.Covers, ", "))
			}
			sb.WriteString(fmt.Sprintf("### Step %d: %s `%s`%s\n\n", i+1, step.Action, step.File, covers))
			if step.Content != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", step.Content))
			}
		}
	}

	// Validation warnings — only WARN and FAIL
	if result.Validation != nil {
		hasIssues := false
		for _, d := range result.Validation.Dimensions {
			if d.Verdict == "WARN" || d.Verdict == "FAIL" {
				hasIssues = true
				break
			}
		}
		if hasIssues {
			sb.WriteString("## Validation Warnings\n\n")
			for _, d := range result.Validation.Dimensions {
				if d.Verdict == "PASS" {
					continue
				}
				icon := "WARN"
				if d.Verdict == "FAIL" {
					icon = "FAIL"
				}
				sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s\n", icon, d.Name, d.Detail))
			}
			sb.WriteString("\n")
		}
		if len(result.Validation.Risks) > 0 {
			sb.WriteString("**Risks**:\n")
			for _, r := range result.Validation.Risks {
				sb.WriteString(fmt.Sprintf("- %s\n", r))
			}
			sb.WriteString("\n")
		}
		if len(result.Validation.MissingFromPlan) > 0 {
			sb.WriteString("**Also address**:\n")
			for _, m := range result.Validation.MissingFromPlan {
				sb.WriteString(fmt.Sprintf("- %s\n", m))
			}
			sb.WriteString("\n")
		}
	}

	// Codebase context — what the agent needs to know but can't discover fast
	if result.Investigation != nil {
		// Existing endpoints — prevent duplicates
		if len(result.Investigation.ExistingEndpoints) > 0 {
			sb.WriteString("## Existing Endpoints (DO NOT recreate)\n\n")
			for _, ep := range result.Investigation.ExistingEndpoints {
				sb.WriteString(fmt.Sprintf("- `%s %s` → %s\n", ep.Method, ep.Path, ep.Class))
			}
			sb.WriteString("\n")
		}

		// Facades — reuse, don't duplicate
		if len(result.Investigation.ExternalAPIs) > 0 {
			sb.WriteString("## Existing Facades (reuse these)\n\n")
			for _, api := range result.Investigation.ExternalAPIs {
				sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", api.FacadeClass, api.File))
			}
			sb.WriteString("\n")
		}

		// Patterns
		if len(result.Investigation.Patterns) > 0 {
			sb.WriteString(fmt.Sprintf("**Patterns**: %s\n\n", strings.Join(result.Investigation.Patterns, ", ")))
		}

		// Call chain
		if len(result.Investigation.CallChain) > 0 {
			sb.WriteString("**Call chain**: ")
			var chain []string
			for _, link := range result.Investigation.CallChain {
				chain = append(chain, fmt.Sprintf("%s → %s", link.From, link.To))
			}
			sb.WriteString(strings.Join(chain, " → "))
			sb.WriteString("\n\n")
		}

		// Related classes — just names and files, the agent reads them itself
		if len(result.Investigation.RelatedClasses) > 0 {
			sb.WriteString("## Related Classes\n\n")
			for _, c := range result.Investigation.RelatedClasses {
				stereo := ""
				if c.Stereotype != "" {
					stereo = " [" + c.Stereotype + "]"
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s — `%s`\n", c.Name, stereo, c.File))
			}
			sb.WriteString("\n")
		}

		// Figma
		if len(result.Investigation.FigmaDesigns) > 0 {
			sb.WriteString("## Figma\n\n")
			for _, fd := range result.Investigation.FigmaDesigns {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", fd.FileName, fd.URL))
				if fd.ScreenAnalysis != nil {
					for _, screen := range fd.ScreenAnalysis.Screens {
						sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n", screen.FrameName, screen.ScreenType, screen.Summary))
						for _, ec := range screen.EdgeCases {
							sb.WriteString(fmt.Sprintf("    - [%s] %s → %s\n", ec.Category, ec.Condition, ec.UIState))
						}
					}
				}
				if len(fd.UIText) > 0 {
					sb.WriteString("  **UI Text:** ")
					limit := len(fd.UIText)
					if limit > 20 {
						limit = 20
					}
					var quoted []string
					for _, t := range fd.UIText[:limit] {
						text := strings.TrimSpace(t)
						if text == "" {
							continue
						}
						if len(text) > 80 {
							text = text[:80] + "..."
						}
						quoted = append(quoted, "\""+text+"\"")
					}
					sb.WriteString(strings.Join(quoted, ", "))
					if len(fd.UIText) > 20 {
						sb.WriteString(fmt.Sprintf(" ... (%d more)", len(fd.UIText)-20))
					}
					sb.WriteString("\n")
				}
			}
			sb.WriteString("\n")
		}

		// Linked issues
		if len(result.Investigation.LinkedIssues) > 0 {
			sb.WriteString("## Linked Issues\n\n")
			for _, li := range result.Investigation.LinkedIssues {
				sb.WriteString(fmt.Sprintf("- **%s** [%s]: %s (status: %s)\n", li.Key, li.Type, li.Summary, li.Status))
				if len(li.Comments) > 0 {
					for _, c := range li.Comments {
						body := c.Body
						if len(body) > 500 {
							body = body[:500] + "... (truncated)"
						}
						sb.WriteString(fmt.Sprintf("  > **%s** (%s):\n  > %s\n\n",
							c.Author, c.Created, strings.ReplaceAll(body, "\n", "\n  > ")))
					}
				}
			}
			sb.WriteString("\n")
		}

		// Library APIs — shared dependency classes available to use
		if len(result.Investigation.LibraryAPIs) > 0 {
			sb.WriteString("## Shared Library APIs (use these, do NOT reinvent)\n\n")
			for _, lib := range result.Investigation.LibraryAPIs {
				sb.WriteString(fmt.Sprintf("**%s:%s:%s**\n", lib.GroupID, lib.ArtifactID, lib.Version))
				for _, cls := range lib.Classes {
					sb.WriteString(fmt.Sprintf("- `%s` — fields: ", cls.Name))
					var fields []string
					for _, f := range cls.Fields {
						fields = append(fields, f)
					}
					sb.WriteString(strings.Join(fields, "; "))
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	// Unknowns — last, so the agent can make assumptions
	if result.Analysis != nil && len(result.Analysis.Unknowns) > 0 {
		sb.WriteString("## Open Questions\n\n")
		sb.WriteString("Make reasonable assumptions for these. Document your assumptions in code comments.\n\n")
		for _, u := range result.Analysis.Unknowns {
			sb.WriteString(fmt.Sprintf("- %s\n", u))
		}
		sb.WriteString("\n")
	}

	// Compact instructions — the agent doesn't need verbose guidance
	sb.WriteString("## Rules\n\n")
	sb.WriteString("1. Read all files listed above before editing anything\n")
	sb.WriteString("2. Follow detected patterns (reactive, caching, etc.)\n")
	sb.WriteString("3. Reuse existing facades — do NOT create duplicates\n")
	sb.WriteString("4. Address all validation warnings and risks\n")
	sb.WriteString("5. Write tests for new/modified code\n")
	sb.WriteString("6. Only code commit, no PR\n")

	return sb.String()
}

// collectFilesToRead extracts unique files from the plan that should be read
// before making changes — edit targets and related classes referenced in steps.
func collectFilesToRead(result *WorkResult) []string {
	seen := make(map[string]bool)
	var files []string

	if result.Plan != nil {
		for _, step := range result.Plan.Steps {
			if step.Action == "edit" && !seen[step.File] {
				seen[step.File] = true
				files = append(files, step.File)
			}
		}
	}

	// Add key related classes not already in the plan
	if result.Investigation != nil {
		for _, c := range result.Investigation.RelatedClasses {
			if !seen[c.File] && isKeyClass(c) {
				seen[c.File] = true
				files = append(files, c.File)
			}
		}
	}

	return files
}

func isKeyClass(c ClassDetail) bool {
	switch c.Stereotype {
	case "service", "rest_controller", "repository", "facade", "configuration":
		return true
	}
	return false
}

func hasMultipleServices(steps []PlanStep) bool {
	seen := ""
	for _, s := range steps {
		if s.Service == "" {
			continue
		}
		if seen == "" {
			seen = s.Service
		} else if s.Service != seen {
			return true
		}
	}
	return false
}

func branchName(result *WorkResult) string {
	if result.Branch != "" {
		return result.Branch
	}
	return "bugfix/" + result.TicketKey
}

// FormatContext produces a lean context bundle from investigation data only.
// No LLM-generated plans, validation, or requirements — just code intelligence
// and raw ticket data for the agent to reason over.
func FormatContext(ticketKey string, ticketType TicketType, issue *jira.Issue, investigation *InvestigationContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s — %s\n\n", ticketKey, string(ticketType)))

	// Target repos
	if investigation != nil && len(investigation.Services) > 0 {
		sb.WriteString("## Target\n\n")
		if len(investigation.ServicePaths) > 1 {
			sb.WriteString("**Multi-repo ticket** — changes needed in multiple repositories:\n\n")
			for _, svcKey := range investigation.Services {
				path := investigation.ServicePaths[svcKey]
				if path == "" {
					continue
				}
				primary := ""
				if svcKey == investigation.Services[0] {
					primary = " **(primary)**"
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s → `%s`\n", svcKey, primary, path))
			}
		} else {
			primary := investigation.Services[0]
			sb.WriteString(fmt.Sprintf("Service: `%s`\n", primary))
			if investigation.RepoPath != "" {
				sb.WriteString(fmt.Sprintf("Path: `%s`\n", investigation.RepoPath))
			}
		}
		sb.WriteString(fmt.Sprintf("Branch: `feature/%s`\n\n", ticketKey))
	}

	// Raw ticket data — the agent extracts requirements itself
	sb.WriteString("## Ticket\n\n")
	sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", issue.Summary))
	if issue.AccCriteria != "" {
		sb.WriteString("**Acceptance Criteria:**\n")
		sb.WriteString(issue.AccCriteria + "\n\n")
	}
	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 4000 {
			desc = desc[:4000] + "\n... (truncated)"
		}
		sb.WriteString("**Description:**\n")
		sb.WriteString(desc + "\n\n")
	}

	// Jira comments — PO clarifications, scope changes, tech notes
	if len(issue.Comments) > 0 {
		sb.WriteString("**Comments:**\n\n")
		limit := len(issue.Comments)
		if limit > 10 {
			sb.WriteString(fmt.Sprintf("_(showing 10 most recent of %d)_\n\n", limit))
			limit = 10
		}
		for i := 0; i < limit; i++ {
			c := issue.Comments[i]
			body := c.Body
			if len(body) > 1000 {
				body = body[:1000] + "... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("> **%s** (%s):\n> %s\n\n",
				c.Author, c.Created, strings.ReplaceAll(body, "\n", "\n> ")))
		}
	}

	// Files to read — collected from investigation's key classes
	if investigation != nil {
		readFiles := collectContextFiles(investigation)
		if len(readFiles) > 0 {
			sb.WriteString("## Read These Files First\n\n")
			for _, f := range readFiles {
				sb.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			sb.WriteString("\n")
		}

		// Existing endpoints
		if len(investigation.ExistingEndpoints) > 0 {
			sb.WriteString("## Existing Endpoints (DO NOT recreate)\n\n")
			for _, ep := range investigation.ExistingEndpoints {
				sb.WriteString(fmt.Sprintf("- `%s %s` → %s\n", ep.Method, ep.Path, ep.Class))
			}
			sb.WriteString("\n")
		}

		// Facades
		if len(investigation.ExternalAPIs) > 0 {
			sb.WriteString("## Existing Facades (reuse these)\n\n")
			for _, api := range investigation.ExternalAPIs {
				sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", api.FacadeClass, api.File))
			}
			sb.WriteString("\n")
		}

		// Patterns
		if len(investigation.Patterns) > 0 {
			sb.WriteString(fmt.Sprintf("**Patterns**: %s\n\n", strings.Join(investigation.Patterns, ", ")))
		}

		// Call chain
		if len(investigation.CallChain) > 0 {
			sb.WriteString("**Call chain**: ")
			var chain []string
			for _, link := range investigation.CallChain {
				chain = append(chain, fmt.Sprintf("%s → %s", link.From, link.To))
			}
			sb.WriteString(strings.Join(chain, " → "))
			sb.WriteString("\n\n")
		}

		// Related classes
		if len(investigation.RelatedClasses) > 0 {
			sb.WriteString("## Related Classes\n\n")
			for _, c := range investigation.RelatedClasses {
				stereo := ""
				if c.Stereotype != "" {
					stereo = " [" + c.Stereotype + "]"
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s — `%s`\n", c.Name, stereo, c.File))
			}
			sb.WriteString("\n")
		}

		// Figma
		if len(investigation.FigmaDesigns) > 0 {
			sb.WriteString("## Figma\n\n")
			for _, fd := range investigation.FigmaDesigns {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", fd.FileName, fd.URL))
				if fd.ScreenAnalysis != nil {
					for _, screen := range fd.ScreenAnalysis.Screens {
						sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n", screen.FrameName, screen.ScreenType, screen.Summary))
					}
				}
				if len(fd.UIText) > 0 {
					sb.WriteString("  **UI Text:** ")
					limit := len(fd.UIText)
					if limit > 20 {
						limit = 20
					}
					var quoted []string
					for _, t := range fd.UIText[:limit] {
						text := strings.TrimSpace(t)
						if text == "" {
							continue
						}
						if len(text) > 80 {
							text = text[:80] + "..."
						}
						quoted = append(quoted, "\""+text+"\"")
					}
					sb.WriteString(strings.Join(quoted, ", "))
					if len(fd.UIText) > 20 {
						sb.WriteString(fmt.Sprintf(" ... (%d more)", len(fd.UIText)-20))
					}
					sb.WriteString("\n")
				}
			}
			sb.WriteString("\n")
		}

		// Linked issues
		if len(investigation.LinkedIssues) > 0 {
			sb.WriteString("## Linked Issues\n\n")
			for _, li := range investigation.LinkedIssues {
				sb.WriteString(fmt.Sprintf("- **%s** [%s]: %s (status: %s)\n", li.Key, li.Type, li.Summary, li.Status))
				if len(li.Comments) > 0 {
					for _, c := range li.Comments {
						body := c.Body
						if len(body) > 500 {
							body = body[:500] + "... (truncated)"
						}
						sb.WriteString(fmt.Sprintf("  > **%s** (%s):\n  > %s\n\n",
							c.Author, c.Created, strings.ReplaceAll(body, "\n", "\n  > ")))
					}
				}
			}
			sb.WriteString("\n")
		}

		// Library APIs
		if len(investigation.LibraryAPIs) > 0 {
			sb.WriteString("## Shared Library APIs (use these, do NOT reinvent)\n\n")
			for _, lib := range investigation.LibraryAPIs {
				sb.WriteString(fmt.Sprintf("**%s:%s:%s**\n", lib.GroupID, lib.ArtifactID, lib.Version))
				for _, cls := range lib.Classes {
					sb.WriteString(fmt.Sprintf("- `%s` — fields: ", cls.Name))
					var fields []string
					for _, f := range cls.Fields {
						fields = append(fields, f)
					}
					sb.WriteString(strings.Join(fields, "; "))
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("## Rules\n\n")
	sb.WriteString("1. Read all files listed above before editing anything\n")
	sb.WriteString("2. Follow detected patterns (reactive, caching, etc.)\n")
	sb.WriteString("3. Reuse existing facades — do NOT create duplicates\n")
	sb.WriteString("4. Write tests for new/modified code\n")
	sb.WriteString("5. Only code commit, no PR\n")

	return sb.String()
}

// collectContextFiles gathers files worth reading from investigation data.
func collectContextFiles(investigation *InvestigationContext) []string {
	seen := make(map[string]bool)
	var files []string

	for _, c := range investigation.RelatedClasses {
		if !seen[c.File] && isKeyClass(c) {
			seen[c.File] = true
			files = append(files, c.File)
		}
	}

	return files
}
