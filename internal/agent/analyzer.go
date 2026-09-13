package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
)

func AnalyzeTicket(llmProvider llm.Provider, issue *jira.Issue, ticketType TicketType, investigation *InvestigationContext) (*AnalysisResult, int, error) {
	systemPrompt := `You are a tech lead doing requirements analysis before implementation planning.

Your job is to THINK, not plan. Read the ticket deeply. Extract every requirement — explicit and implied. Identify every edge case and state transition. Flag unknowns.

Output ONLY valid JSON:
{
  "requirements": [
    {
      "id": "REQ-1",
      "description": "what must be built or handled",
      "category": "core|integration|error-handling|caching|security|ui-state|config|testing",
      "code_impact": "what code change this implies (new class, new field, new facade call, config entry, etc.)",
      "priority": "must|should|nice|existing"
    }
  ],
  "edge_cases": [
    {
      "trigger": "what condition causes this state",
      "expected_behavior": "what the system should do",
      "code_impact": "what code handles this"
    }
  ],
  "unknowns": ["questions that need answers before implementation"]
}

Rules:
- Read the FULL ticket description and acceptance criteria word by word. Every sentence may contain a requirement.
- "never show X" → that's an error-handling requirement with a specific edge case
- "when X is unavailable" → that's a degradation edge case
- Brand mentions (multiple brands) → that's a multi-tenant requirement
- Timestamps, refresh intervals, caching hints → those are caching requirements
- "hide on ships that don't support" → that's a feature-gate requirement
- API names (PTF, COH, Siebel) → each is an integration requirement with its own error/timeout edge case
- Figma screens showing different states → each state is a UI-state requirement
- Don't skip requirements because they seem obvious. If the ticket says it, extract it.
- After extracting all requirements, CONSOLIDATE: group requirements that can be satisfied by the same code change into a single requirement. For example, "24hr TTL", "server-side enforcement", and "expired section" might all be satisfied by a single filtering method — merge them into one requirement with a combined description.
- For Bug tickets, minimize the number of requirements. A bug fix typically has 1-3 core requirements, not 15. If you extracted more than 5 for a Bug, consolidate harder.
- Mark requirements that are already handled by existing code as "priority": "existing" — the planner should skip these.`

	var ctx strings.Builder

	// Give the LLM the codebase context so it can map requirements to code
	if len(investigation.ExistingEndpoints) > 0 {
		ctx.WriteString("Existing endpoints in this service:\n")
		for _, ep := range investigation.ExistingEndpoints {
			ctx.WriteString(fmt.Sprintf("  %s %s → %s\n", ep.Method, ep.Path, ep.Class))
		}
	}
	if len(investigation.ExternalAPIs) > 0 {
		ctx.WriteString("\nExisting facades (outbound dependencies):\n")
		for _, api := range investigation.ExternalAPIs {
			ctx.WriteString(fmt.Sprintf("  %s (%s)\n", api.FacadeClass, api.File))
		}
	}
	if len(investigation.Patterns) > 0 {
		ctx.WriteString(fmt.Sprintf("\nCodebase patterns: %s\n", strings.Join(investigation.Patterns, ", ")))
	}
	for _, fd := range investigation.FigmaDesigns {
		if len(fd.Frames) > 0 {
			ctx.WriteString(fmt.Sprintf("\nFigma screens: %s\n", strings.Join(fd.Frames, ", ")))
		}
		if len(fd.UIText) > 0 {
			ctx.WriteString(fmt.Sprintf("Figma UI text: %s\n", strings.Join(fd.UIText, " | ")))
		}
		if fd.ScreenAnalysis != nil {
			ctx.WriteString("\n--- Deep Figma Screen Analysis ---\n")
			for _, screen := range fd.ScreenAnalysis.Screens {
				ctx.WriteString(fmt.Sprintf("\nScreen: %s (type=%s)\n", screen.FrameName, screen.ScreenType))
				ctx.WriteString(fmt.Sprintf("  Summary: %s\n", screen.Summary))
				for _, ec := range screen.EdgeCases {
					ctx.WriteString(fmt.Sprintf("  Edge case [%s]: %s → %s\n", ec.Category, ec.Condition, ec.UIState))
				}
				for _, interaction := range screen.Interactions {
					ctx.WriteString(fmt.Sprintf("  Interaction: %s → %s", interaction.Trigger, interaction.Action))
					if interaction.SavesTo != "" {
						ctx.WriteString(fmt.Sprintf(" (saves to: %s)", interaction.SavesTo))
					}
					ctx.WriteString("\n")
				}
			}
			if len(fd.ScreenAnalysis.BackendNeeds) > 0 {
				ctx.WriteString("\nInferred backend needs:\n")
				for _, need := range fd.ScreenAnalysis.BackendNeeds {
					ctx.WriteString(fmt.Sprintf("  %s %s — %s\n", need.Method, need.Endpoint, need.Description))
				}
			}
			if len(fd.ScreenAnalysis.FrontendLogic) > 0 {
				ctx.WriteString("\nInferred frontend logic:\n")
				for _, fl := range fd.ScreenAnalysis.FrontendLogic {
					ctx.WriteString(fmt.Sprintf("  %s: %s\n", fl.Logic, fl.Description))
				}
			}
			if len(fd.ScreenAnalysis.OpenQuestions) > 0 {
				ctx.WriteString("\nOpen questions from screen analysis:\n")
				for _, q := range fd.ScreenAnalysis.OpenQuestions {
					ctx.WriteString(fmt.Sprintf("  ? %s\n", q))
				}
			}
			ctx.WriteString("--- End Figma Analysis ---\n")
		}
	}
	if len(investigation.LinkedIssues) > 0 {
		ctx.WriteString("\nLinked issues:\n")
		for _, li := range investigation.LinkedIssues {
			ctx.WriteString(fmt.Sprintf("  [%s] %s: %s (status: %s)\n", li.Type, li.Key, li.Summary, li.Status))
		}
	}

	// Platform awareness
	if investigation.TicketPlatform != "" && investigation.TicketPlatform != PlatformBackend {
		ctx.WriteString(fmt.Sprintf("\n⚠ PLATFORM: This is a %s ticket.\n", investigation.TicketPlatform))
		if !investigation.PlatformRepoFound {
			ctx.WriteString(fmt.Sprintf("⚠ NO %s REPO INDEXED — codebase context below is backend services only.\n", investigation.TicketPlatform))
			ctx.WriteString("Focus requirements on: what the app needs from backend APIs, what API contracts must exist, what error responses the app expects.\n")
			ctx.WriteString("Do NOT generate backend implementation requirements — this is a frontend ticket.\n")
		}
		if len(investigation.BackendAPIs) > 0 {
			ctx.WriteString("\nBackend API endpoints available for the app to consume:\n")
			for _, ep := range investigation.BackendAPIs {
				ctx.WriteString(fmt.Sprintf("  %s %s → %s (%s)\n", ep.Method, ep.Path, ep.Class, ep.File))
			}
		}
	}
	for _, w := range investigation.Warnings {
		ctx.WriteString(fmt.Sprintf("⚠ %s\n", w))
	}

	userPrompt := fmt.Sprintf(`Ticket: %s
Type: %s
Summary: %s

Description:
%s

Acceptance Criteria:
%s

Components: %s
Labels: %s

Codebase Context:
%s

Analyze this ticket completely. Extract ALL requirements, edge cases, and unknowns.`,
		issue.Key, ticketType, issue.Summary,
		issue.Description,
		issue.AccCriteria,
		strings.Join(issue.Components, ", "),
		strings.Join(issue.Labels, ", "),
		ctx.String(),
	)

	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    llmProvider.MaxOutputTokens(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("analysis failed: %w", err)
	}

	var result AnalysisResult
	content := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		content = cleanJSON(content)
		if retryErr := json.Unmarshal([]byte(content), &result); retryErr != nil {
			return nil, resp.TokensUsed, fmt.Errorf("failed to parse analysis JSON: %w\nRaw (first 500): %.500s", err, resp.Content)
		}
	}

	return &result, resp.TokensUsed, nil
}

func FormatAnalysis(a *AnalysisResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Requirements: %d extracted\n", len(a.Requirements)))

	// Group by category
	cats := make(map[string][]Requirement)
	for _, r := range a.Requirements {
		cats[r.Category] = append(cats[r.Category], r)
	}
	for cat, reqs := range cats {
		sb.WriteString(fmt.Sprintf("\n  [%s]\n", strings.ToUpper(cat)))
		for _, r := range reqs {
			priority := ""
			if r.Priority == "must" {
				priority = " *"
			}
			sb.WriteString(fmt.Sprintf("    %s: %s%s\n", r.ID, r.Description, priority))
			sb.WriteString(fmt.Sprintf("         -> %s\n", r.CodeImpact))
		}
	}

	if len(a.EdgeCases) > 0 {
		sb.WriteString(fmt.Sprintf("\nEdge Cases: %d identified\n", len(a.EdgeCases)))
		for _, ec := range a.EdgeCases {
			sb.WriteString(fmt.Sprintf("  When: %s\n", ec.Trigger))
			sb.WriteString(fmt.Sprintf("  Then: %s\n", ec.Expected))
			sb.WriteString(fmt.Sprintf("    -> %s\n", ec.CodeImpact))
		}
	}

	if len(a.Unknowns) > 0 {
		sb.WriteString(fmt.Sprintf("\nUnknowns: %d questions\n", len(a.Unknowns)))
		for _, u := range a.Unknowns {
			sb.WriteString(fmt.Sprintf("  ? %s\n", u))
		}
	}

	return sb.String()
}

func analysisToPromptSection(analysis *AnalysisResult) string {
	var sb strings.Builder

	sb.WriteString("\nAnalyzed Requirements (all MUST be covered — group related ones into shared steps):\n")
	for _, r := range analysis.Requirements {
		sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", r.Priority, r.ID, r.Description))
		sb.WriteString(fmt.Sprintf("    Code impact: %s\n", r.CodeImpact))
	}

	if len(analysis.EdgeCases) > 0 {
		sb.WriteString("\nEdge Cases (handle within the plan steps above — do not create new steps just for edge cases):\n")
		for i, ec := range analysis.EdgeCases {
			sb.WriteString(fmt.Sprintf("  EC-%d: When %s → %s\n", i+1, ec.Trigger, ec.Expected))
			sb.WriteString(fmt.Sprintf("    Code: %s\n", ec.CodeImpact))
		}
	}

	if len(analysis.Unknowns) > 0 {
		sb.WriteString("\nUnknowns (note assumptions made):\n")
		for _, u := range analysis.Unknowns {
			sb.WriteString(fmt.Sprintf("  ? %s\n", u))
		}
	}

	return sb.String()
}
