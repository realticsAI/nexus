package figma

import (
	"testing"
)

func TestAnalyzeScreen_FormScreen(t *testing.T) {
	screen := &Node{
		ID: "1:1", Name: "App Preferences", Type: "FRAME",
		Children: []*Node{
			{ID: "1:2", Name: "Back Arrow", Type: "VECTOR"},
			{ID: "1:3", Name: "Page Title", Type: "TEXT", Characters: "App Preferences"},
			{ID: "1:4", Name: "Description", Type: "TEXT", Characters: "Tell us what you like"},
			{ID: "1:5", Name: "Game Preferences Segment", Type: "INSTANCE",
				Children: []*Node{
					{ID: "1:6", Name: "Slots", Type: "TEXT", Characters: "Slots"},
					{ID: "1:7", Name: "Both", Type: "TEXT", Characters: "Both"},
					{ID: "1:8", Name: "Table games", Type: "TEXT", Characters: "Table games"},
				},
			},
			{ID: "1:9", Name: "Toggle Last Minute", Type: "INSTANCE",
				Children: []*Node{
					{ID: "1:10", Name: "Label", Type: "TEXT", Characters: "Interested in last-minute offers"},
					{ID: "1:11", Name: "Switch", Type: "FRAME"},
				},
			},
			{ID: "1:12", Name: "Favorite Ships Row", Type: "FRAME",
				Children: []*Node{
					{ID: "1:13", Name: "Title", Type: "TEXT", Characters: "Favorite Ships"},
					{ID: "1:14", Name: "Caption", Type: "TEXT", Characters: "Select up to 5"},
					{ID: "1:15", Name: "Chevron", Type: "VECTOR"},
				},
			},
			{ID: "1:16", Name: "Update Button CTA", Type: "FRAME",
				Children: []*Node{
					{ID: "1:17", Name: "Label", Type: "TEXT", Characters: "Update preferences"},
				},
			},
		},
	}

	analysis := AnalyzeScreen(screen)

	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
	if analysis.FrameName != "App Preferences" {
		t.Errorf("expected frame name 'App Preferences', got %q", analysis.FrameName)
	}
	if analysis.ScreenType != "form" {
		t.Errorf("expected screen type 'form', got %q", analysis.ScreenType)
	}

	// Should detect elements
	if len(analysis.Elements) == 0 {
		t.Fatal("expected elements to be detected")
	}

	// Should have edge cases
	if len(analysis.EdgeCases) == 0 {
		t.Fatal("expected edge cases to be detected")
	}

	// Check for resilience edge cases
	hasResilience := false
	for _, ec := range analysis.EdgeCases {
		if ec.Category == "resilience" {
			hasResilience = true
			break
		}
	}
	if !hasResilience {
		t.Error("expected resilience edge cases")
	}

	// Check for validation edge case (select up to 5)
	hasValidation := false
	for _, ec := range analysis.EdgeCases {
		if ec.Category == "validation" {
			hasValidation = true
			break
		}
	}
	if !hasValidation {
		t.Error("expected validation edge case from 'Select up to 5' text")
	}

	// Should have interactions
	if len(analysis.Interactions) == 0 {
		t.Error("expected interactions to be detected")
	}

	t.Logf("Screen type: %s", analysis.ScreenType)
	t.Logf("Elements: %d", len(analysis.Elements))
	t.Logf("Edge cases: %d", len(analysis.EdgeCases))
	t.Logf("Interactions: %d", len(analysis.Interactions))
	t.Logf("Data sources: %d", len(analysis.DataSources))
	t.Logf("Summary: %s", analysis.Summary)
}

func TestAnalyzeScreen_ErrorScreen(t *testing.T) {
	screen := &Node{
		ID: "2:1", Name: "Error State", Type: "FRAME",
		Children: []*Node{
			{ID: "2:2", Name: "Error Icon", Type: "VECTOR"},
			{ID: "2:3", Name: "Error Title", Type: "TEXT", Characters: "Something went wrong"},
			{ID: "2:4", Name: "Error Message", Type: "TEXT", Characters: "We couldn't load your preferences"},
			{ID: "2:5", Name: "Retry Button", Type: "FRAME",
				Children: []*Node{
					{ID: "2:6", Name: "Label", Type: "TEXT", Characters: "Try again"},
				},
			},
		},
	}

	analysis := AnalyzeScreen(screen)
	if analysis.ScreenType != "error" {
		t.Errorf("expected 'error' screen type, got %q", analysis.ScreenType)
	}

	hasRetry := false
	for _, ec := range analysis.EdgeCases {
		if ec.Category == "resilience" {
			hasRetry = true
		}
	}
	if !hasRetry {
		t.Error("expected resilience edge case for error screen")
	}
}

func TestAnalyzeScreen_ListScreen(t *testing.T) {
	screen := &Node{
		ID: "3:1", Name: "Select Ships", Type: "FRAME",
		Children: []*Node{
			{ID: "3:2", Name: "Header", Type: "TEXT", Characters: "Select your favorite ships"},
			{ID: "3:3", Name: "Ship Row Symphony", Type: "FRAME",
				Children: []*Node{
					{ID: "3:4", Name: "Ship Image", Type: "FRAME"},
					{ID: "3:5", Name: "Ship Name", Type: "TEXT", Characters: "Symphony of the Seas"},
					{ID: "3:6", Name: "Checkbox", Type: "VECTOR"},
				},
			},
			{ID: "3:7", Name: "Ship Row Oasis", Type: "FRAME",
				Children: []*Node{
					{ID: "3:8", Name: "Ship Name", Type: "TEXT", Characters: "Oasis of the Seas"},
				},
			},
			{ID: "3:9", Name: "Apply Button", Type: "FRAME",
				Children: []*Node{
					{ID: "3:10", Name: "Label", Type: "TEXT", Characters: "Apply"},
				},
			},
		},
	}

	analysis := AnalyzeScreen(screen)
	if analysis.ScreenType != "list" {
		t.Errorf("expected 'list' screen type, got %q", analysis.ScreenType)
	}

	hasEmptyState := false
	for _, ec := range analysis.EdgeCases {
		if ec.Category == "empty_state" {
			hasEmptyState = true
		}
	}
	if !hasEmptyState {
		t.Error("expected empty_state edge case for list screen")
	}
}

func TestAnalyzeAllScreens(t *testing.T) {
	root := &Node{
		ID: "0:0", Name: "Document", Type: "DOCUMENT",
		Children: []*Node{
			{ID: "0:1", Name: "Preferences", Type: "CANVAS",
				Children: []*Node{
					{ID: "1:1", Name: "Main Form", Type: "FRAME",
						Children: []*Node{
							{ID: "1:2", Name: "Game Segment Control", Type: "INSTANCE"},
							{ID: "1:3", Name: "Toggle Offers", Type: "INSTANCE"},
							{ID: "1:4", Name: "Save Button CTA", Type: "FRAME",
								Children: []*Node{
									{ID: "1:5", Name: "Label", Type: "TEXT", Characters: "Update preferences"},
								},
							},
						},
					},
					{ID: "2:1", Name: "Ship List", Type: "FRAME",
						Children: []*Node{
							{ID: "2:2", Name: "Ship Row", Type: "FRAME"},
							{ID: "2:3", Name: "Apply Button", Type: "FRAME",
								Children: []*Node{
									{ID: "2:4", Name: "Label", Type: "TEXT", Characters: "Apply"},
								},
							},
						},
					},
					{ID: "3:1", Name: "Error Oops", Type: "FRAME",
						Children: []*Node{
							{ID: "3:2", Name: "Error Title", Type: "TEXT", Characters: "Something went wrong"},
							{ID: "3:3", Name: "Retry Button", Type: "FRAME",
								Children: []*Node{
									{ID: "3:4", Name: "Label", Type: "TEXT", Characters: "Try again"},
								},
							},
						},
					},
				},
			},
		},
	}

	result := AnalyzeAllScreens(root)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Screens) != 3 {
		t.Fatalf("expected 3 screens, got %d", len(result.Screens))
	}

	// Should have backend needs
	if len(result.BackendNeeds) == 0 {
		t.Error("expected backend needs to be inferred")
	}

	// Should have frontend logic
	if len(result.FrontendLogic) == 0 {
		t.Error("expected frontend logic to be inferred")
	}

	// Should have data flow map
	if len(result.DataFlowMap) == 0 {
		t.Error("expected data flow map")
	}

	t.Logf("Screens: %d", len(result.Screens))
	t.Logf("Backend needs: %d", len(result.BackendNeeds))
	t.Logf("Frontend logic: %d", len(result.FrontendLogic))
	t.Logf("Data flow map: %d entries", len(result.DataFlowMap))
	t.Logf("Open questions: %d", len(result.OpenQuestions))

	for _, screen := range result.Screens {
		t.Logf("  Screen: %s (type=%s, elements=%d, edge_cases=%d)",
			screen.FrameName, screen.ScreenType, len(screen.Elements), len(screen.EdgeCases))
	}
}

func TestClassifyScreen(t *testing.T) {
	tests := []struct {
		name     string
		text     []string
		expected string
	}{
		{"Error State", []string{"Something went wrong", "Try again"}, "error"},
		{"Ship List", []string{"Select your ships", "Apply"}, "list"},
		{"Preferences", []string{"Update preferences", "Save your settings"}, "form"},
		{"Loading", []string{"Loading", "Please wait"}, "loading"},
		{"Empty", []string{"No results found", "Nothing here yet"}, "empty"},
	}

	for _, tc := range tests {
		result := classifyScreen(tc.name, tc.text)
		if result != tc.expected {
			t.Errorf("classifyScreen(%q) = %q, want %q", tc.name, result, tc.expected)
		}
	}
}
