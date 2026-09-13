package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anurag/nexus/internal/gitops"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
	"github.com/anurag/nexus/internal/security"
)

type ReviewFinding struct {
	File     string `json:"file"`
	Severity string `json:"severity"`
	Issue    string `json:"issue"`
	Fix      string `json:"fix"`
}

type ReviewResult struct {
	Verdict  string          `json:"verdict"`
	Findings []ReviewFinding `json:"findings"`
	Fixes    []ReviewFix     `json:"fixes,omitempty"`
}

type ReviewFix struct {
	File string `json:"file"`
	Code string `json:"code"`
}

func ReviewGeneratedCode(
	llmProvider llm.Provider,
	issue *jira.Issue,
	ticketType TicketType,
	investigation *InvestigationContext,
	analysis *AnalysisResult,
	plan *Plan,
	g *gitops.GitOps,
	filesChanged []string,
) (*ReviewResult, int, error) {
	systemPrompt := `You are a SENIOR ARCHITECT reviewing AI-generated code for a production codebase.

You receive: the original Jira ticket, investigation context (existing classes, endpoints, patterns, Figma designs), the implementation plan, and the actual code that was generated.

Your job:
1. Check every generated file compiles and follows the codebase patterns
2. Verify the code actually solves the ticket — not just touches files
3. Find bugs: null pointers, missing imports, wrong method names, type mismatches
4. Check for commented-out code, dead code, or TODO stubs that should be real code
5. Verify no duplicate logic across files
6. Check field/method names match EXACTLY what exists in the codebase (from investigation context)

Output ONLY valid JSON:
{
  "verdict": "PASS|WARN|FAIL",
  "findings": [
    {
      "file": "path/to/file.java",
      "severity": "CRITICAL|HIGH|MEDIUM|LOW",
      "issue": "description of the problem",
      "fix": "what should be done"
    }
  ],
  "fixes": [
    {
      "file": "path/to/file.java",
      "code": "the corrected complete file content"
    }
  ]
}

RULES:
- verdict is FAIL if ANY finding is CRITICAL
- verdict is WARN if findings exist but none CRITICAL
- verdict is PASS if code is production-ready
- Only include "fixes" for CRITICAL and HIGH findings where you are confident in the fix
- Each fix must contain the COMPLETE corrected file content, not a patch
- Reference actual class names and fields from the investigation context`

	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("# Ticket: %s\n", issue.Key))
	prompt.WriteString(fmt.Sprintf("Summary: %s\n", issue.Summary))
	prompt.WriteString(fmt.Sprintf("Type: %s\n", ticketType))
	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 2000 {
			desc = desc[:2000] + "\n... truncated"
		}
		prompt.WriteString(fmt.Sprintf("Description:\n%s\n", desc))
	}
	if issue.AccCriteria != "" {
		prompt.WriteString(fmt.Sprintf("\nAcceptance Criteria:\n%s\n", issue.AccCriteria))
	}

	if analysis != nil {
		prompt.WriteString("\n# Requirements (from analysis phase — already extracted, saves you re-reading the ticket)\n")
		for _, r := range analysis.Requirements {
			prompt.WriteString(fmt.Sprintf("  %s [%s]: %s\n", r.ID, r.Priority, r.Description))
		}
		if len(analysis.EdgeCases) > 0 {
			prompt.WriteString("Edge cases:\n")
			for _, e := range analysis.EdgeCases {
				prompt.WriteString(fmt.Sprintf("  - %s → %s\n", e.Trigger, e.Expected))
			}
		}
	}

	if investigation != nil {
		prompt.WriteString("\n# Codebase Context (from investigation phase — already extracted)\n")

		if len(investigation.ExistingEndpoints) > 0 {
			prompt.WriteString("Existing endpoints:\n")
			for _, ep := range investigation.ExistingEndpoints {
				prompt.WriteString(fmt.Sprintf("  %s %s → %s\n", ep.Method, ep.Path, ep.Class))
			}
		}

		if len(investigation.Patterns) > 0 {
			prompt.WriteString(fmt.Sprintf("Patterns: %s\n", strings.Join(investigation.Patterns, ", ")))
		}

		if len(investigation.FileExcerpts) > 0 {
			prompt.WriteString("\n# Related Class Signatures (ground truth — check generated code against these)\n")
			for _, e := range investigation.FileExcerpts {
				safe := security.RedactSecrets(e.Signature)
				if len(safe) > 1500 {
					safe = safe[:1500] + "\n// ... truncated"
				}
				prompt.WriteString(fmt.Sprintf("\n[CLASS] %s (%s):\n%s\n", e.ClassName, e.File, safe))
			}
		}

		if len(investigation.FigmaDesigns) > 0 {
			prompt.WriteString("\n# Figma Designs (from investigation — already extracted)\n")
			for _, fd := range investigation.FigmaDesigns {
				prompt.WriteString(fmt.Sprintf("  File: %s\n", fd.FileName))
				if len(fd.UIText) > 0 {
					prompt.WriteString(fmt.Sprintf("  UI Text: %s\n", strings.Join(fd.UIText, ", ")))
				}
				if len(fd.ScreenStates) > 0 {
					prompt.WriteString(fmt.Sprintf("  States: %s\n", strings.Join(fd.ScreenStates, ", ")))
				}
			}
		}
	}

	if plan != nil {
		prompt.WriteString("\n# Implementation Plan\n")
		prompt.WriteString(fmt.Sprintf("Summary: %s\n", plan.Summary))
		for i, s := range plan.Steps {
			prompt.WriteString(fmt.Sprintf("  Step %d: %s %s\n", i+1, s.Action, s.File))
		}
	}

	prompt.WriteString("\n# Generated Code (REVIEW THIS)\n")

	for _, file := range filesChanged {
		content, err := g.ReadFile(file)
		if err != nil {
			prompt.WriteString(fmt.Sprintf("\n--- %s (could not read: %v) ---\n", file, err))
			continue
		}
		safe := security.RedactSecrets(content)
		if len(safe) > 5000 {
			safe = safe[:5000] + "\n// ... truncated"
		}
		prompt.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", file, safe))
	}

	diff, _ := g.Diff()
	if diff != "" {
		if len(diff) > 8000 {
			diff = diff[:8000] + "\n... truncated"
		}
		prompt.WriteString(fmt.Sprintf("\n# Git Diff (changes vs main):\n%s\n", diff))
	}

	prompt.WriteString("\nReview this code as a senior architect. Output JSON only.")

	start := time.Now()
	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   prompt.String(),
		MaxTokens:    llmProvider.MaxOutputTokens(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("review LLM call failed: %w", err)
	}

	body := resp.Content
	body = strings.TrimPrefix(body, "```json")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	var result ReviewResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, resp.TokensUsed, fmt.Errorf("review response parse error: %w\nraw: %s", err, truncate(body, 500))
	}

	_ = start
	return &result, resp.TokensUsed, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func BuildAgentDirectPrompt(
	issue *jira.Issue,
	ticketType TicketType,
	investigation *InvestigationContext,
	analysis *AnalysisResult,
	plan *Plan,
	diff string,
) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Review Nexus-generated code for %s\n\n", issue.Key))
	b.WriteString(fmt.Sprintf("Ticket: %s — %s\n", issue.Key, issue.Summary))
	b.WriteString(fmt.Sprintf("Type: %s\n\n", ticketType))

	if issue.Description != "" {
		desc := issue.Description
		if len(desc) > 3000 {
			desc = desc[:3000] + "\n... truncated"
		}
		b.WriteString(fmt.Sprintf("## Ticket Description\n%s\n\n", desc))
	}
	if issue.AccCriteria != "" {
		b.WriteString(fmt.Sprintf("## Acceptance Criteria\n%s\n\n", issue.AccCriteria))
	}

	if investigation != nil && len(investigation.FigmaDesigns) > 0 {
		b.WriteString("## Figma Designs\n")
		for _, fd := range investigation.FigmaDesigns {
			b.WriteString(fmt.Sprintf("- %s (%s)\n", fd.FileName, fd.URL))
			if len(fd.UIText) > 0 {
				b.WriteString(fmt.Sprintf("  UI Text: %s\n", strings.Join(fd.UIText, " | ")))
			}
			if fd.ScreenAnalysis != nil {
				for _, s := range fd.ScreenAnalysis.Screens {
					b.WriteString(fmt.Sprintf("  Screen: %s (%s)\n", s.FrameName, s.ScreenType))
				}
				if len(fd.ScreenAnalysis.BackendNeeds) > 0 {
					var needs []string
					for _, bn := range fd.ScreenAnalysis.BackendNeeds {
						needs = append(needs, fmt.Sprintf("%s %s: %s", bn.Method, bn.Endpoint, bn.Description))
					}
					b.WriteString(fmt.Sprintf("  Backend needs: %s\n", strings.Join(needs, "; ")))
				}
			}
		}
		b.WriteString("\n")
	}

	if analysis != nil && len(analysis.Requirements) > 0 {
		b.WriteString("## Requirements\n")
		for _, r := range analysis.Requirements {
			b.WriteString(fmt.Sprintf("- %s [%s]: %s\n", r.ID, r.Priority, r.Description))
		}
		b.WriteString("\n")
	}

	if investigation != nil && len(investigation.Patterns) > 0 {
		b.WriteString(fmt.Sprintf("## Codebase Patterns\n%s\n\n", strings.Join(investigation.Patterns, ", ")))
	}

	if investigation != nil && len(investigation.FileExcerpts) > 0 {
		b.WriteString("## Key Class Signatures\n")
		for _, e := range investigation.FileExcerpts {
			sig := e.Signature
			if len(sig) > 1000 {
				sig = sig[:1000] + "\n// truncated"
			}
			b.WriteString(fmt.Sprintf("### %s (%s)\n```\n%s\n```\n\n", e.ClassName, e.File, sig))
		}
	}

	if plan != nil {
		b.WriteString("## Plan\n")
		for i, s := range plan.Steps {
			b.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, s.Action, s.File))
		}
		b.WriteString("\n")
	}

	if diff != "" {
		if len(diff) > 10000 {
			diff = diff[:10000] + "\n... truncated"
		}
		b.WriteString(fmt.Sprintf("## Code Changes (git diff)\n```diff\n%s\n```\n\n", diff))
	}

	b.WriteString("## Instructions\n")
	b.WriteString("You are a senior architect. Nexus (an AI agent) generated this code.\n")
	b.WriteString("Review every changed file. Fix any bugs, missing imports, wrong method names, dead code.\n")
	b.WriteString("Make the code fully working and production-ready. Compile-check mentally.\n")
	b.WriteString("Only edit the files that need fixes — don't touch working code.\n")

	return b.String()
}
