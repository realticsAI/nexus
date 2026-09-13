package agent

import (
	"fmt"
	"strings"

	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
)

func GenerateTD(llmProvider llm.Provider, issue *jira.Issue, ticketType TicketType, investigation *InvestigationContext, analysis *AnalysisResult, plan *Plan, validation *ValidationResult) (string, int, error) {
	systemPrompt := `You are a senior software architect writing a Technical Design document.

You receive: a Jira ticket, codebase investigation (existing classes, endpoints, facades, call chains, patterns), extracted requirements, an implementation plan, and validation feedback.

Write a COMPLETE Technical Design document in Markdown. This is the document that gets reviewed before any code is written. It must be precise enough that any developer can implement from it.

Structure:

# Technical Design — {ticket key}: {summary}

## 1. Overview
- Business context (what and why)
- Scope (what's in, what's out)
- Ticket type and priority

## 2. Current Architecture
- Affected services and their roles
- Existing endpoints (with paths and controllers)
- Existing facades and outbound dependencies
- Current call chain (controller → service → repository → facade)
- Detected patterns (reactive, caching, headers, etc.)

## 3. Requirements Summary
- Table of all requirements with ID, description, priority, and code impact
- Edge cases that MUST be handled
- Open questions / assumptions

## 4. Proposed Design

### 4.1 Component Design
For each new or modified component:
- Class name and file path
- Responsibility
- Key methods with signatures
- Dependencies (injected services/facades)

### 4.2 API Contract
For each new or modified endpoint:
- HTTP method and path
- Request headers required
- Request body schema
- Response body schema
- Error responses

### 4.3 Data Model
- New entities or schema changes
- Couchbase/database document structure changes
- Cache key patterns and TTLs

### 4.4 Integration Points
- External API calls (facades to reuse, new facade calls needed)
- Internal service calls
- Kafka events (produce/consume)

## 5. Error Handling & Degradation
- Per-integration failure handling (what happens when each downstream fails)
- Timeout and retry strategy
- Fallback behavior
- Circuit breaker considerations

## 6. Security
- Authentication/authorization changes
- Input validation
- Sensitive data handling

## 7. Testing Strategy
- Unit tests (which classes, which scenarios)
- Integration tests
- Contract tests (if API changes)
- Edge case test scenarios from requirements

## 8. Risks & Mitigations
- Technical risks from validation
- Dependency risks
- Performance considerations
- Migration/rollback plan

## 9. Implementation Sequence
- Ordered list of changes with dependencies between them
- What can be done in parallel
- Estimated complexity per component

Rules:
- Reference ACTUAL class names, file paths, and method signatures from the codebase context
- Do NOT invent classes that don't exist — clearly mark NEW vs EXISTING
- Every requirement MUST appear in the design
- Every validation warning MUST be addressed
- Use the existing architectural patterns — do not introduce new patterns
- Be specific about error handling — "handle errors gracefully" is not a design
- Include actual request/response JSON examples where endpoints change`

	var ctx strings.Builder

	// Requirements
	if analysis != nil {
		ctx.WriteString("## Analyzed Requirements\n\n")
		for _, r := range analysis.Requirements {
			ctx.WriteString(fmt.Sprintf("- [%s] %s: %s (code: %s)\n", r.Priority, r.ID, r.Description, r.CodeImpact))
		}
		if len(analysis.EdgeCases) > 0 {
			ctx.WriteString("\n## Edge Cases\n\n")
			for i, ec := range analysis.EdgeCases {
				ctx.WriteString(fmt.Sprintf("- EC-%d: When %s → %s (code: %s)\n", i+1, ec.Trigger, ec.Expected, ec.CodeImpact))
			}
		}
		if len(analysis.Unknowns) > 0 {
			ctx.WriteString("\n## Unknowns\n\n")
			for _, u := range analysis.Unknowns {
				ctx.WriteString(fmt.Sprintf("- %s\n", u))
			}
		}
	}

	// Existing endpoints
	if len(investigation.ExistingEndpoints) > 0 {
		ctx.WriteString("\n## Existing Endpoints\n\n")
		for _, ep := range investigation.ExistingEndpoints {
			ctx.WriteString(fmt.Sprintf("- %s %s → %s (%s)\n", ep.Method, ep.Path, ep.Class, ep.File))
		}
	}

	// Facades
	if len(investigation.ExternalAPIs) > 0 {
		ctx.WriteString("\n## Existing Facades\n\n")
		for _, api := range investigation.ExternalAPIs {
			label := "EXTERNAL"
			if api.Internal {
				label = "INTERNAL"
			}
			ctx.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", label, api.FacadeClass, api.File))
			for _, u := range api.URLs {
				ctx.WriteString(fmt.Sprintf("  → %s\n", u))
			}
		}
	}

	// Call chain
	if len(investigation.CallChain) > 0 {
		ctx.WriteString("\n## Call Chain\n\n")
		for _, link := range investigation.CallChain {
			ctx.WriteString(fmt.Sprintf("- %s → %s (via %s)\n", link.From, link.To, link.Via))
		}
	}

	// Patterns
	if len(investigation.Patterns) > 0 {
		ctx.WriteString(fmt.Sprintf("\n## Architectural Patterns\n\n%s\n", strings.Join(investigation.Patterns, ", ")))
	}

	// File signatures
	if len(investigation.FileExcerpts) > 0 {
		ctx.WriteString("\n## Key Class Signatures\n\n")
		for _, e := range investigation.FileExcerpts {
			ctx.WriteString(fmt.Sprintf("### %s (%s)\n```java\n%s\n```\n\n", e.ClassName, e.File, e.Signature))
		}
	}

	// Figma
	for _, fd := range investigation.FigmaDesigns {
		ctx.WriteString(fmt.Sprintf("\n## Figma: %s\n", fd.FileName))
		if len(fd.Frames) > 0 {
			ctx.WriteString(fmt.Sprintf("Screens: %s\n", strings.Join(fd.Frames, ", ")))
		}
		if len(fd.UIText) > 0 {
			ctx.WriteString(fmt.Sprintf("UI Text: %s\n", strings.Join(fd.UIText, " | ")))
		}
	}

	// Linked issues
	if len(investigation.LinkedIssues) > 0 {
		ctx.WriteString("\n## Linked Issues\n\n")
		for _, li := range investigation.LinkedIssues {
			ctx.WriteString(fmt.Sprintf("- [%s] %s: %s (status: %s, assignee: %s)\n",
				li.Type, li.Key, li.Summary, li.Status, li.Assignee))
		}
	}

	// Plan
	if plan != nil {
		ctx.WriteString(fmt.Sprintf("\n## Implementation Plan (%d steps)\n\n", len(plan.Steps)))
		ctx.WriteString(fmt.Sprintf("Summary: %s\n\n", plan.Summary))
		for i, step := range plan.Steps {
			covers := ""
			if len(step.Covers) > 0 {
				covers = fmt.Sprintf(" [%s]", strings.Join(step.Covers, ", "))
			}
			ctx.WriteString(fmt.Sprintf("%d. %s %s%s\n", i+1, step.Action, step.File, covers))
		}
	}

	// Validation feedback
	if validation != nil {
		ctx.WriteString(fmt.Sprintf("\n## Validation Verdict: %s\n\n", validation.Verdict))
		for _, d := range validation.Dimensions {
			ctx.WriteString(fmt.Sprintf("- [%s] %s: %s\n", d.Verdict, d.Name, d.Detail))
		}
		if len(validation.Risks) > 0 {
			ctx.WriteString("\n### Risks\n")
			for _, r := range validation.Risks {
				ctx.WriteString(fmt.Sprintf("- %s\n", r))
			}
		}
		if len(validation.Suggestions) > 0 {
			ctx.WriteString("\n### Suggestions\n")
			for _, s := range validation.Suggestions {
				ctx.WriteString(fmt.Sprintf("- %s\n", s))
			}
		}
		if len(validation.MissingFromPlan) > 0 {
			ctx.WriteString("\n### Missing from Plan\n")
			for _, m := range validation.MissingFromPlan {
				ctx.WriteString(fmt.Sprintf("- %s\n", m))
			}
		}
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

%s

Write the complete Technical Design document. Every requirement must appear. Every validation warning must be addressed. Reference actual class names and file paths.`,
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
		return "", 0, fmt.Errorf("TD generation failed: %w", err)
	}

	return resp.Content, resp.TokensUsed, nil
}
