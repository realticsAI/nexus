package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
	"github.com/anurag/nexus/internal/security"
)

func BuildPlan(llmProvider llm.Provider, issue *jira.Issue, ticketType TicketType, investigation *InvestigationContext, analysis *AnalysisResult) (*Plan, int, error) {
	systemPrompt := `You are a senior software engineer. Given a ticket, analyzed requirements, and codebase investigation context, produce a JSON implementation plan.

Output ONLY valid JSON with this structure:
{"summary": "one-line description", "steps": [{"action": "create|edit", "file": "relative/path", "content": "full file content or edit instructions", "covers": ["REQ-1", "REQ-3"]}]}

CRITICAL RULES:
1. Every requirement in the "Analyzed Requirements" section MUST be covered by at least one step. Use the "covers" field to map steps to requirement IDs.
2. Every edge case MUST be handled — add error handling, fallback logic, or state fields as needed.
3. Do NOT create new classes that duplicate existing ones. Check "Existing Facades" and "Existing Endpoints" before creating anything.
4. Follow the detected architectural patterns exactly (reactive Mono/Flux, WebClient, Redis caching, request headers).
5. Use the class signatures and call chain to understand the exact code structure.
6. Use Figma UI text and screen states to understand the required user-facing changes.
7. Keep changes MINIMAL. For Bug tickets, touch the FEWEST files possible — prefer editing existing classes over creating new ones. A single well-placed change is better than scattered changes across many files.
8. SCOPE BUDGET: Bug tickets should plan max 8 source files + tests. Feature tickets max 15. If you find yourself planning more, consolidate.
9. Multiple requirements CAN and SHOULD share a single plan step when they affect the same file or the same logical unit. Do NOT create one step per requirement — group related changes.
10. Do NOT duplicate logic across multiple files. If TTL filtering logic exists in a mapper, do NOT also add it to the service, the facade, and the controller. Single source of truth.
11. Do NOT add commented-out code, placeholder code, or feature flag classes with no implementation. Every file you create or edit must contain working, compilable code.
12. Prefer using existing Spring/framework abstractions (@ConfigurationProperties, @Cacheable, @CacheEvict) over injecting specific implementations (RedisTemplate, CouchbaseTemplate). Follow the abstractions already in the codebase.`

	var sections strings.Builder

	// Analyzed requirements — the key addition
	if analysis != nil {
		sections.WriteString(analysisToPromptSection(analysis))
	}

	// Call chain
	if len(investigation.CallChain) > 0 {
		sections.WriteString("\nCall Chain:\n")
		for _, link := range investigation.CallChain {
			sections.WriteString(fmt.Sprintf("  %s → %s (via %s)\n", link.From, link.To, link.Via))
		}
	}

	// Related classes: ranked by structural importance, capped to reduce noise
	primaryService := ""
	if len(investigation.Services) > 0 {
		primaryService = investigation.Services[0]
	}
	if len(investigation.RelatedClasses) > 0 {
		// Score and rank classes by structural importance
		type rankedClass struct {
			cd    ClassDetail
			score int
		}
		var ranked []rankedClass
		for _, cd := range investigation.RelatedClasses {
			if primaryService != "" && cd.Service != primaryService {
				continue
			}
			s := 0
			switch cd.Stereotype {
			case "rest_controller":
				s += 50
			case "service":
				s += 40
			case "component":
				s += 30
			case "repository":
				s += 25
			case "configuration":
				s += 20
			default:
				s += 5
			}
			s += len(cd.Methods) * 2
			s += len(cd.Endpoints) * 10
			if len(cd.Headers) > 0 {
				s += 5
			}
			// Boost classes whose methods reference key DTOs mentioned in file excerpts
			for _, m := range cd.Methods {
				mLower := strings.ToLower(m)
				for _, ex := range investigation.FileExcerpts {
					if strings.Contains(mLower, strings.ToLower(ex.ClassName)) {
						s += 15
						break
					}
				}
			}
			ranked = append(ranked, rankedClass{cd, s})
		}
		// Sort by score descending
		for i := 0; i < len(ranked); i++ {
			for j := i + 1; j < len(ranked); j++ {
				if ranked[j].score > ranked[i].score {
					ranked[i], ranked[j] = ranked[j], ranked[i]
				}
			}
		}
		cap := 50
		if ticketType == Bug {
			cap = 20
		}
		if len(ranked) < cap {
			cap = len(ranked)
		}

		sections.WriteString(fmt.Sprintf("\nRelated Classes (%d most relevant of %d):\n", cap, len(ranked)))
		for _, rc := range ranked[:cap] {
			cd := rc.cd
			sections.WriteString(fmt.Sprintf("  [%s] %s (%s)\n", cd.Stereotype, cd.Name, cd.File))
			if len(cd.Annotations) > 0 {
				sections.WriteString(fmt.Sprintf("    Annotations: %s\n", strings.Join(cd.Annotations, ", ")))
			}
			for _, m := range cd.Methods {
				sections.WriteString(fmt.Sprintf("    - %s\n", m))
			}
			if len(cd.Endpoints) > 0 {
				sections.WriteString(fmt.Sprintf("    Endpoints: %s\n", strings.Join(cd.Endpoints, ", ")))
			}
			if len(cd.Headers) > 0 {
				sections.WriteString(fmt.Sprintf("    Request Headers: %s\n", strings.Join(cd.Headers, ", ")))
			}
		}
	}

	// File signatures (actual Java code structure) — redact secrets before sending to LLM
	if len(investigation.FileExcerpts) > 0 {
		sections.WriteString("\nFile Signatures (actual code):\n")
		for _, e := range investigation.FileExcerpts {
			safe := security.RedactSecrets(e.Signature)
			sections.WriteString(fmt.Sprintf("--- %s (%s) ---\n%s\n", e.ClassName, e.File, safe))
		}
	}

	// Library APIs — shared dependency classes the LLM must know about
	if len(investigation.LibraryAPIs) > 0 {
		sections.WriteString("\nShared Library APIs (from Maven dependencies — use these, do NOT reinvent):\n")
		for _, lib := range investigation.LibraryAPIs {
			sections.WriteString(fmt.Sprintf("--- %s:%s:%s ---\n", lib.GroupID, lib.ArtifactID, lib.Version))
			for _, cls := range lib.Classes {
				sections.WriteString(fmt.Sprintf("  class %s.%s {\n", cls.Package, cls.Name))
				for _, f := range cls.Fields {
					sections.WriteString(fmt.Sprintf("    %s\n", f))
				}
				for _, m := range cls.Methods {
					sections.WriteString(fmt.Sprintf("    %s\n", m))
				}
				sections.WriteString("  }\n")
			}
		}
	}

	// Type consumers: trace which services/classes use the key DTOs
	// This helps the LLM find ALL files that need changes when a DTO gets a new field
	// Gated to Feature/Refactor — Bug tickets should minimize scope, not expand it
	if ticketType != Bug && len(investigation.FileExcerpts) > 0 && len(investigation.RelatedClasses) > 0 {
		dtoNames := map[string]bool{}
		for _, cd := range investigation.RelatedClasses {
			nameLower := strings.ToLower(cd.Name)
			if strings.Contains(nameLower, "request") || strings.Contains(nameLower, "response") ||
				strings.Contains(nameLower, "dto") || strings.Contains(nameLower, "itinerary") ||
				strings.Contains(nameLower, "sailing") || strings.Contains(nameLower, "entity") {
				if cd.Stereotype == "" || cd.Stereotype == "model" {
					dtoNames[cd.Name] = true
				}
			}
		}
		if len(dtoNames) > 0 {
			consumers := map[string][]string{}
			for _, cd := range investigation.RelatedClasses {
				if cd.Stereotype != "service" && cd.Stereotype != "rest_controller" && cd.Stereotype != "component" {
					continue
				}
				for _, m := range cd.Methods {
					for dtoName := range dtoNames {
						if strings.Contains(m, dtoName) {
							consumers[dtoName] = appendUnique(consumers[dtoName], fmt.Sprintf("%s.%s", cd.Name, extractMethodName(m)))
						}
					}
				}
			}
			if len(consumers) > 0 {
				sections.WriteString("\nType Consumers (classes that build/use key DTOs — if a DTO gains a field, ALL consumers may need updating):\n")
				for dto, users := range consumers {
					sections.WriteString(fmt.Sprintf("  %s is used by:\n", dto))
					for _, u := range users {
						sections.WriteString(fmt.Sprintf("    → %s\n", u))
					}
				}
			}
		}
	}

	// Existing endpoints (DO NOT recreate these)
	if len(investigation.ExistingEndpoints) > 0 {
		sections.WriteString("\nExisting Endpoints (already implemented — DO NOT recreate):\n")
		for _, ep := range investigation.ExistingEndpoints {
			sections.WriteString(fmt.Sprintf("  %s %s → %s (%s)\n", ep.Method, ep.Path, ep.Class, ep.File))
		}
	}

	// Linked issues (context from related tickets)
	if len(investigation.LinkedIssues) > 0 {
		sections.WriteString("\nLinked Issues (context — check status before duplicating work):\n")
		for _, li := range investigation.LinkedIssues {
			sections.WriteString(fmt.Sprintf("  [%s] %s: %s (assignee: %s, status: %s)\n", li.Type, li.Key, li.Summary, li.Assignee, li.Status))
		}
	}

	// External API calls (outbound dependencies from facades)
	if len(investigation.ExternalAPIs) > 0 {
		sections.WriteString("\nExisting Facades (reuse these — DO NOT create duplicates):\n")
		for _, api := range investigation.ExternalAPIs {
			label := "EXTERNAL"
			if api.Internal {
				label = "INTERNAL"
			}
			sections.WriteString(fmt.Sprintf("  [%s] %s (%s)\n", label, api.FacadeClass, api.File))
			for _, u := range api.URLs {
				sections.WriteString(fmt.Sprintf("    → %s\n", u))
			}
			for _, c := range api.ConfigKeys {
				sections.WriteString(fmt.Sprintf("    config: %s\n", c))
			}
		}
	}

	// Architectural patterns
	if len(investigation.Patterns) > 0 {
		sections.WriteString(fmt.Sprintf("\nArchitectural Patterns (MUST follow): %s\n", strings.Join(investigation.Patterns, ", ")))
	}

	// Existing files on disk
	if len(investigation.ExistingFiles) > 0 {
		sections.WriteString(fmt.Sprintf("\nExisting Files (on disk): %d files confirmed\n", len(investigation.ExistingFiles)))
		for _, f := range investigation.ExistingFiles {
			sections.WriteString(fmt.Sprintf("  %s\n", f))
		}
	}

	// Figma designs with deep screen analysis
	for _, fd := range investigation.FigmaDesigns {
		sections.WriteString(fmt.Sprintf("\nFigma Design: %s (%s)\n", fd.FileName, fd.URL))
		if len(fd.Frames) > 0 {
			sections.WriteString(fmt.Sprintf("  Frames/Screens: %s\n", strings.Join(fd.Frames, ", ")))
		}
		if fd.ScreenAnalysis != nil {
			for _, screen := range fd.ScreenAnalysis.Screens {
				sections.WriteString(fmt.Sprintf("\n  [%s] %s\n", screen.ScreenType, screen.FrameName))
				for _, ec := range screen.EdgeCases {
					sections.WriteString(fmt.Sprintf("    Edge case [%s]: %s → %s\n", ec.Category, ec.Condition, ec.UIState))
				}
				for _, ds := range screen.DataSources {
					sections.WriteString(fmt.Sprintf("    Data: %s ← %s (%s)\n", ds.Field, ds.Source, ds.Description))
				}
			}
			for _, need := range fd.ScreenAnalysis.BackendNeeds {
				sections.WriteString(fmt.Sprintf("  Backend: %s %s — %s\n", need.Method, need.Endpoint, need.Description))
			}
			for _, fl := range fd.ScreenAnalysis.FrontendLogic {
				sections.WriteString(fmt.Sprintf("  Frontend: %s — %s\n", fl.Logic, fl.Description))
			}
		} else {
			if len(fd.UIText) > 0 {
				sections.WriteString(fmt.Sprintf("  UI Text: %s\n", strings.Join(fd.UIText, " | ")))
			}
		}
	}

	// Platform awareness
	if investigation.TicketPlatform != "" && investigation.TicketPlatform != PlatformBackend {
		sections.WriteString(fmt.Sprintf("\n⚠ PLATFORM: This is a %s ticket.\n", investigation.TicketPlatform))
		if !investigation.PlatformRepoFound {
			sections.WriteString(fmt.Sprintf("⚠ NO %s REPO INDEXED. The codebase context above is backend-only.\n", investigation.TicketPlatform))
			sections.WriteString("DO NOT generate backend Java/Kotlin/Swift code changes.\n")
			sections.WriteString("Instead, the plan should focus on:\n")
			sections.WriteString("  1. What backend API endpoints the app needs (check if they already exist)\n")
			sections.WriteString("  2. What API contract gaps exist (missing fields, missing endpoints)\n")
			sections.WriteString("  3. What error responses the app should handle\n")
			sections.WriteString("  4. Frontend component/screen implementation (describe structure, not code — no repo to target)\n")
			sections.WriteString("  5. Flag that the platform repo must be indexed for actual code generation\n")
		}
		if len(investigation.BackendAPIs) > 0 {
			sections.WriteString("\nBackend API endpoints available for the app:\n")
			for _, ep := range investigation.BackendAPIs {
				sections.WriteString(fmt.Sprintf("  %s %s → %s (%s)\n", ep.Method, ep.Path, ep.Class, ep.File))
			}
		}
	}
	for _, w := range investigation.Warnings {
		sections.WriteString(fmt.Sprintf("⚠ %s\n", w))
	}

	userPrompt := fmt.Sprintf(`Ticket: %s
Type: %s
Summary: %s
Description: %s
Components: %s
Acceptance Criteria: %s

Investigation Context:
- Services: %s
- Endpoints: %s
- Dependencies: %s
%s
Create an implementation plan that covers EVERY analyzed requirement. Map each step to the requirements it addresses using the "covers" field.`,
		issue.Key, ticketType, issue.Summary, issue.Description,
		strings.Join(issue.Components, ", "),
		issue.AccCriteria,
		strings.Join(investigation.Services, ", "),
		strings.Join(investigation.Endpoints, ", "),
		strings.Join(investigation.Dependencies, ", "),
		sections.String(),
	)

	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    llmProvider.MaxOutputTokens(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("plan generation failed: %w", err)
	}

	var plan Plan
	content := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		content = cleanJSON(content)
		if retryErr := json.Unmarshal([]byte(content), &plan); retryErr != nil {
			return nil, resp.TokensUsed, fmt.Errorf("failed to parse plan JSON: %w\nRaw (first 500): %.500s", err, resp.Content)
		}
	}

	return &plan, resp.TokensUsed, nil
}

func extractJSON(s string) string {
	s = stripMarkdownFences(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = s[7:]
	} else if strings.HasPrefix(s, "```") {
		s = s[3:]
	}
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}

func cleanJSON(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")

	// Fix trailing commas before } or ]
	for _, pair := range [][2]string{{",}", "}"}, {",]", "]"}} {
		for strings.Contains(s, pair[0]) {
			s = strings.ReplaceAll(s, pair[0], pair[1])
		}
	}
	// Fix comma-space variants
	for _, pair := range [][2]string{{", }", "}"}, {", ]", "]"}} {
		for strings.Contains(s, pair[0]) {
			s = strings.ReplaceAll(s, pair[0], pair[1])
		}
	}
	return s
}

func PatchPlan(llmProvider llm.Provider, plan *Plan, gaps []string, investigation *InvestigationContext) ([]PlanStep, int, error) {
	systemPrompt := `You are a senior software engineer. You are given an existing implementation plan and a list of gaps identified by a validator. Generate ONLY the additional plan steps needed to fill these gaps.

Output ONLY valid JSON with this structure:
{"steps": [{"action": "create|edit", "file": "relative/path", "content": "edit instructions", "covers": ["REQ-1"]}]}

RULES:
1. Do NOT repeat or duplicate existing steps. Only generate NEW steps for the gaps.
2. Use the Available Classes list to find the correct file paths — do not invent paths.
3. Follow the same style and detail level as the existing plan steps.
4. Each gap must be addressed by at least one step.
5. For test file gaps: include what to test, mock setup, and assertions.
6. For observability gaps: include specific log levels, message formats, and where to add them.`

	var prompt strings.Builder

	prompt.WriteString("Existing Plan Steps (DO NOT duplicate these):\n")
	for i, step := range plan.Steps {
		prompt.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, step.Action, step.File))
	}

	prompt.WriteString("\nValidator Gaps (MUST address each one):\n")
	for i, g := range gaps {
		prompt.WriteString(fmt.Sprintf("  %d. %s\n", i+1, g))
	}

	if len(investigation.RelatedClasses) > 0 {
		prompt.WriteString("\nAvailable Classes (use for correct file paths):\n")
		for _, cd := range investigation.RelatedClasses {
			if cd.Stereotype == "service" || cd.Stereotype == "rest_controller" ||
				cd.Stereotype == "component" || cd.Stereotype == "repository" ||
				strings.Contains(strings.ToLower(cd.Name), "test") {
				prompt.WriteString(fmt.Sprintf("  [%s] %s (%s)\n", cd.Stereotype, cd.Name, cd.File))
			}
		}
	}

	if len(investigation.ExistingFiles) > 0 {
		prompt.WriteString("\nTest Files on Disk:\n")
		for _, f := range investigation.ExistingFiles {
			if strings.Contains(f, "Test") || strings.Contains(f, "test") {
				prompt.WriteString(fmt.Sprintf("  %s\n", f))
			}
		}
	}

	prompt.WriteString("\nGenerate ONLY the additional steps to fill the gaps above.")

	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   prompt.String(),
		MaxTokens:    llmProvider.MaxOutputTokens(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("patch plan failed: %w", err)
	}

	var patch struct {
		Steps []PlanStep `json:"steps"`
	}
	content := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(content), &patch); err != nil {
		content = cleanJSON(content)
		if retryErr := json.Unmarshal([]byte(content), &patch); retryErr != nil {
			return nil, resp.TokensUsed, fmt.Errorf("failed to parse patch JSON: %w\nRaw (first 500): %.500s", err, resp.Content)
		}
	}

	return patch.Steps, resp.TokensUsed, nil
}

func extractMethodName(sig string) string {
	// "Mono<List<Foo>> fetchSailingsForToday(String brand, ...)" → "fetchSailingsForToday"
	paren := strings.Index(sig, "(")
	if paren > 0 {
		sig = sig[:paren]
	}
	parts := strings.Fields(sig)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return sig
}
