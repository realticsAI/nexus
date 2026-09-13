package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
)

func ValidatePlan(llmProvider llm.Provider, issue *jira.Issue, ticketType TicketType, investigation *InvestigationContext, plan *Plan) (*ValidationResult, int, error) {
	systemPrompt := `You are a senior architect reviewing an AI-generated implementation plan against codebase reality.

You receive: the original ticket, investigation context (existing classes, endpoints, facades, patterns, Figma designs), and the proposed plan.

Evaluate each dimension and output ONLY valid JSON:
{
  "verdict": "PASS|WARN|FAIL",
  "dimensions": [
    {
      "name": "dimension name",
      "verdict": "PASS|WARN|FAIL",
      "detail": "explanation"
    }
  ],
  "risks": ["risk descriptions"],
  "suggestions": ["actionable improvements"],
  "missing_from_plan": ["things the ticket requires but the plan doesn't address"]
}

Dimensions to evaluate:
1. SERVICE_TARGET — Is the correct service/module targeted?
2. ENDPOINT_OVERLAP — Does the plan recreate any existing endpoints?
3. FACADE_REUSE — Does the plan create new facades when existing ones could be extended?
4. PATTERN_COMPLIANCE — Does the plan follow detected patterns (reactive, caching, headers)?
5. FIGMA_COVERAGE — Does the plan address all Figma UI screens/states?
6. MODEL_DESIGN — Are response models well-structured and complete?
7. TEST_COVERAGE — Does the plan include appropriate tests?
8. MISSING_CONCERNS — Caching, error handling, brand logic, config management?

Be specific. Reference actual class names and file paths from the investigation context.`

	var ctx strings.Builder

	// Existing endpoints
	if len(investigation.ExistingEndpoints) > 0 {
		ctx.WriteString("Existing Endpoints:\n")
		for _, ep := range investigation.ExistingEndpoints {
			ctx.WriteString(fmt.Sprintf("  %s %s → %s (%s)\n", ep.Method, ep.Path, ep.Class, ep.File))
		}
	}

	// External APIs / facades
	if len(investigation.ExternalAPIs) > 0 {
		ctx.WriteString("\nExisting Facades:\n")
		for _, api := range investigation.ExternalAPIs {
			label := "EXTERNAL"
			if api.Internal {
				label = "INTERNAL"
			}
			ctx.WriteString(fmt.Sprintf("  [%s] %s (%s)\n", label, api.FacadeClass, api.File))
		}
	}

	// Related classes
	if len(investigation.RelatedClasses) > 0 {
		ctx.WriteString("\nRelated Classes:\n")
		for _, cd := range investigation.RelatedClasses {
			ctx.WriteString(fmt.Sprintf("  [%s] %s (%s)\n", cd.Stereotype, cd.Name, cd.File))
			if len(cd.Headers) > 0 {
				ctx.WriteString(fmt.Sprintf("    Headers: %s\n", strings.Join(cd.Headers, ", ")))
			}
		}
	}

	// Patterns
	if len(investigation.Patterns) > 0 {
		ctx.WriteString(fmt.Sprintf("\nDetected Patterns: %s\n", strings.Join(investigation.Patterns, ", ")))
	}

	// Figma
	for _, fd := range investigation.FigmaDesigns {
		ctx.WriteString(fmt.Sprintf("\nFigma Design: %s\n", fd.FileName))
		if len(fd.Frames) > 0 {
			ctx.WriteString(fmt.Sprintf("  Screens: %s\n", strings.Join(fd.Frames, ", ")))
		}
		if len(fd.UIText) > 0 {
			ctx.WriteString(fmt.Sprintf("  UI Text: %s\n", strings.Join(fd.UIText, " | ")))
		}
	}

	// Plan steps
	var planSummary strings.Builder
	planSummary.WriteString(fmt.Sprintf("Plan: %s\n", plan.Summary))
	for i, step := range plan.Steps {
		planSummary.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, step.Action, step.File))
	}

	userPrompt := fmt.Sprintf(`Ticket: %s
Type: %s
Summary: %s
Description: %s
Acceptance Criteria: %s

Investigation Context:
%s

Proposed Plan:
%s

Validate this plan against the investigation context.`,
		issue.Key, ticketType, issue.Summary, issue.Description,
		issue.AccCriteria,
		ctx.String(),
		planSummary.String(),
	)

	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    llmProvider.MaxOutputTokens(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("validation failed: %w", err)
	}

	var result ValidationResult
	content := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		content = cleanJSON(content)
		if retryErr := json.Unmarshal([]byte(content), &result); retryErr != nil {
			return nil, resp.TokensUsed, fmt.Errorf("failed to parse validation JSON: %w\nRaw (first 500): %.500s", err, resp.Content)
		}
	}

	return &result, resp.TokensUsed, nil
}

func FormatValidation(v *ValidationResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Validation: %s\n\n", v.Verdict))

	for _, d := range v.Dimensions {
		icon := "  "
		switch d.Verdict {
		case "PASS":
			icon = "OK"
		case "WARN":
			icon = "!!"
		case "FAIL":
			icon = "XX"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", icon, d.Name, d.Detail))
	}

	if len(v.Risks) > 0 {
		sb.WriteString("\nRisks:\n")
		for _, r := range v.Risks {
			sb.WriteString(fmt.Sprintf("  - %s\n", r))
		}
	}

	if len(v.Suggestions) > 0 {
		sb.WriteString("\nSuggestions:\n")
		for _, s := range v.Suggestions {
			sb.WriteString(fmt.Sprintf("  - %s\n", s))
		}
	}

	if len(v.MissingFromPlan) > 0 {
		sb.WriteString("\nMissing from plan:\n")
		for _, m := range v.MissingFromPlan {
			sb.WriteString(fmt.Sprintf("  - %s\n", m))
		}
	}

	return sb.String()
}
