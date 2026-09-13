package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	mux := http.NewServeMux()

	// ── Jira API ──
	mux.HandleFunc("/rest/api/2/issue/", handleJiraIssue)
	// ── Anthropic Messages API (mock LLM) ──
	mux.HandleFunc("/v1/messages", handleLLM)
	// ── Health check ──
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	port := os.Getenv("MOCK_PORT")
	if port == "" {
		port = "9999"
	}
	fmt.Fprintf(os.Stderr, "mockserver: listening on :%s (Jira + LLM)\n", port)
	http.ListenAndServe(":"+port, mux)
}

func handleJiraIssue(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	key := parts[len(parts)-1]

	if strings.Contains(r.URL.Path, "/remotelink") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	tickets := map[string]map[string]any{
		"PROJ-2001": jiraResponse("PROJ-2001",
			"Commerce | Add reward points display to member dashboard",
			"As a premium member, I want to see my current points balance on the dashboard.\n\nThe dashboard should show:\n- Current points balance from the member account\n- Points expiry date\n- Tier status (Gold, Platinum, Diamond)\n- A CTA to redeem points\n\nThe backend GET /info endpoint already returns the member profile. The iOS app needs to consume the membershipStatus and pointsBalance fields.",
			"Story",
			"1. Points balance is visible on the main dashboard\n2. Points refresh on pull-to-refresh\n3. Expired points show a visual indicator\n4. Tier badge matches the member's current tier\n5. Non-members see an enrollment CTA instead",
			[]string{"commerce", "rewards"},
			[]string{"Commerce"},
		),
		"PROJ-2002": jiraResponse("PROJ-2002",
			"Commerce | Fix null pointer when guest profile is missing",
			"NPE in CustomerProfileService.getProfile() when the guest has no member account.\n\nStack trace:\njava.lang.NullPointerException at CustomerProfileService.java:42\n  at CustomerProfileController.getInfo()\n\nSteps to reproduce:\n1. Call GET /info with a brand-new guest account\n2. Backend returns 500 instead of 404 or empty profile",
			"Bug",
			"1. GET /info returns 404 or empty profile for non-enrolled guests\n2. No 500 error in logs\n3. Existing enrolled guests unaffected",
			[]string{"commerce", "bug"},
			[]string{"Commerce"},
		),
	}

	ticket, ok := tickets[key]
	if !ok {
		ticket = tickets["PROJ-2001"]
		ticket["key"] = key
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticket)
}

func jiraResponse(key, summary, description, issueType, ac string, labels, components []string) map[string]any {
	comps := make([]map[string]string, len(components))
	for i, c := range components {
		comps[i] = map[string]string{"name": c}
	}
	return map[string]any{
		"key": key,
		"fields": map[string]any{
			"summary":     summary,
			"description": description,
			"issuetype":   map[string]string{"name": issueType},
			"status":      map[string]string{"name": "To Do"},
			"priority":    map[string]string{"name": "High"},
			"assignee":    map[string]string{"displayName": "Test User"},
			"labels":      labels,
			"components":  comps,
			"customfield_10001": ac,
			"issuelinks":  []any{},
		},
	}
}

var llmCallCount int

func handleLLM(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	prompt := string(body)
	llmCallCount++

	var response string

	switch {
	case strings.Contains(prompt, "requirements analysis") || strings.Contains(prompt, "Extract ALL requirements"):
		response = analyzeResponse()
	case strings.Contains(prompt, "implementation plan") || strings.Contains(prompt, "Create an implementation plan"):
		response = planResponse()
	case strings.Contains(prompt, "Validate") || strings.Contains(prompt, "validate this plan"):
		response = validateResponse()
	case strings.Contains(prompt, "Technical Design") || strings.Contains(prompt, "technical design document"):
		response = designResponse()
	default:
		response = analyzeResponse()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"content": []map[string]string{{"type": "text", "text": response}},
		"usage":   map[string]int{"input_tokens": 500, "output_tokens": 300},
		"model":   "mock-llm",
	})
}

func analyzeResponse() string {
	result := map[string]any{
		"requirements": []map[string]any{
			{"id": "REQ-1", "description": "Display current reward points balance on main dashboard", "category": "core", "code_impact": "Add pointsBalance field to dashboard view model", "priority": "must"},
			{"id": "REQ-2", "description": "Show points expiry date", "category": "ui-state", "code_impact": "Parse and format expiryDate from GET /info response", "priority": "must"},
			{"id": "REQ-3", "description": "Display tier badge matching member tier", "category": "ui-state", "code_impact": "Map tierLevel enum to badge asset", "priority": "must"},
			{"id": "REQ-4", "description": "Show enrollment CTA for non-members", "category": "ui-state", "code_impact": "Conditional rendering based on membershipStatus", "priority": "should"},
		},
		"edge_cases": []map[string]any{
			{"trigger": "GET /info returns 404 for non-enrolled guest", "expected_behavior": "Show enrollment CTA, not error screen", "code_impact": "Handle 404 as non-member state"},
			{"trigger": "GET /info times out", "expected_behavior": "Show loading skeleton then retry", "code_impact": "Add timeout handler with retry logic"},
			{"trigger": "Points balance is zero", "expected_behavior": "Show 0 points with earn-more CTA", "code_impact": "Zero-state UI variant"},
		},
		"unknowns": []string{
			"What is the exact API contract for the pointsBalance field — integer or decimal?",
			"Are there Figma designs for the tier badge variants?",
		},
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func planResponse() string {
	result := map[string]any{
		"summary": "Add reward points display to main dashboard by consuming GET /info endpoint fields",
		"steps": []map[string]any{
			{"action": "edit", "file": "order.service/src/main/java/com/example/platform/order/service/models/response/CustomerProfileResponse.java", "content": "Add pointsBalance (int), expiryDate (String), tierLevel (String) fields to response model", "covers": []string{"REQ-1", "REQ-2", "REQ-3"}},
			{"action": "edit", "file": "order.service/src/main/java/com/example/platform/order/service/services/CustomerProfileService.java", "content": "Map member account fields to new response fields in getProfile()", "covers": []string{"REQ-1", "REQ-2"}},
			{"action": "edit", "file": "order.service/src/test/java/com/example/platform/order/service/services/CustomerProfileServiceTest.java", "content": "Add test for points balance mapping and null handling", "covers": []string{"REQ-1"}},
			{"action": "create", "file": "order.service/src/test/java/com/example/platform/order/service/services/NonMemberProfileTest.java", "content": "Test that non-enrolled guests get empty profile with membershipStatus=NON_MEMBER", "covers": []string{"REQ-4"}},
		},
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func validateResponse() string {
	result := map[string]any{
		"verdict": "WARN",
		"dimensions": []map[string]any{
			{"name": "SERVICE_TARGET", "verdict": "PASS", "detail": "Plan correctly targets order.service service"},
			{"name": "ENDPOINT_OVERLAP", "verdict": "PASS", "detail": "No duplicate endpoints"},
			{"name": "FACADE_REUSE", "verdict": "PASS", "detail": "Uses existing GuestFacade"},
			{"name": "PATTERN_COMPLIANCE", "verdict": "PASS", "detail": "Maintains reactive Mono/Flux patterns"},
			{"name": "MODEL_DESIGN", "verdict": "WARN", "detail": "tierLevel field should use an enum, not String"},
			{"name": "TEST_COVERAGE", "verdict": "WARN", "detail": "Missing integration test for full endpoint response"},
		},
		"risks": []string{
			"tierLevel as String allows invalid values — use enum",
			"No cache invalidation when points change",
		},
		"suggestions": []string{
			"Add TierLevel enum (GOLD, PLATINUM, DIAMOND, NONE)",
			"Add integration test hitting the full controller",
			"Document points refresh strategy",
		},
		"missing_from_plan": []string{
			"Cache invalidation for points balance updates",
		},
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func designResponse() string {
	return `# Technical Design: Main Dashboard Points Display

## 1. Overview
Add reward points balance, expiry, and tier display to the main dashboard.

## 2. Architecture
Extends existing GET /info endpoint response with three new fields.
No new services or endpoints required.

## 3. Data Model Changes
- CustomerProfileResponse: +pointsBalance (int), +expiryDate (ISO-8601), +tierLevel (TierLevel enum)

## 4. Implementation
Edit CustomerProfileService.getProfile() to map fields from member account.

## 5. Testing
Unit tests for mapping logic. Integration test for full endpoint response.

## 6. Risks
- Cache invalidation when points change
- tierLevel enum backward compatibility
`
}
