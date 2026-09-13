package agent

import (
	"encoding/json"
	"testing"

	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/model"
)

func TestClassifyBug(t *testing.T) {
	issue := &jira.Issue{Type: "Bug", Summary: "Fix payment processing error"}
	if Classify(issue) != Bug {
		t.Fatal("expected Bug")
	}
}

func TestClassifyFeature(t *testing.T) {
	issue := &jira.Issue{Type: "Story", Summary: "Add reward points calculation"}
	if Classify(issue) != Feature {
		t.Fatal("expected Feature")
	}
}

func TestClassifyRefactor(t *testing.T) {
	issue := &jira.Issue{Type: "Task", Summary: "Refactor order processing pipeline"}
	if Classify(issue) != Refactor {
		t.Fatal("expected Refactor")
	}
}

func TestClassifyChore(t *testing.T) {
	issue := &jira.Issue{Type: "Chore", Summary: "Update dependencies"}
	if Classify(issue) != Chore {
		t.Fatal("expected Chore")
	}
}

func TestInvestigate(t *testing.T) {
	issue := &jira.Issue{
		Key:        "PROJ-123",
		Summary:    "Fix payment validation in order service",
		Components: []string{"orders"},
		Labels:     []string{"payment"},
	}

	services := map[string]*model.ServiceIndex{
		"backend:orders": {
			Key: "backend:orders",
			Units: []model.Unit{
				{Name: "OrderController", Type: "class", File: "OrderController.java",
					Endpoints: []string{"POST /api/orders"},
					Methods:   []model.Method{{Name: "createOrder"}}},
				{Name: "PaymentValidator", Type: "class", File: "PaymentValidator.java",
					Methods: []model.Method{{Name: "validatePayment"}}},
			},
		},
		"backend:payments": {
			Key: "backend:payments",
			Units: []model.Unit{
				{Name: "PaymentService", Type: "class", File: "PaymentService.java"},
			},
		},
	}

	graph := &model.Graph{
		Edges: []model.Edge{
			{From: "backend:orders", To: "backend:payments", Type: model.HTTPCall},
		},
	}

	ctx := Investigate(issue, services, graph, "TestOrg", nil)
	if len(ctx.Services) == 0 {
		t.Fatal("expected at least 1 service")
	}
	found := false
	for _, s := range ctx.Services {
		if s == "backend:orders" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected backend:orders in services")
	}
}

func TestExtractJSON(t *testing.T) {
	input := `Here is the plan:
{"summary": "test", "steps": []}
Done.`
	result := extractJSON(input)
	if result != `{"summary": "test", "steps": []}` {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestExtractKeywords(t *testing.T) {
	issue := &jira.Issue{
		Summary:    "Fix payment validation in order service",
		Components: []string{"orders"},
		Labels:     []string{"critical"},
	}
	kw := extractKeywords(issue)
	if len(kw) == 0 {
		t.Fatal("expected keywords")
	}
}

func TestIsStopWord(t *testing.T) {
	if !isStopWord("the") {
		t.Fatal("'the' should be stop word")
	}
	if isStopWord("payment") {
		t.Fatal("'payment' should not be stop word")
	}
}

func TestClassifyPrep(t *testing.T) {
	cases := []struct {
		summary string
		desc    string
		want    TicketType
	}{
		{"Commerce | RN | Feature Preparation | Reward Points", "", Prep},
		{"Commerce | Spike — evaluate caching options", "", Prep},
		{"Research best approach for points display", "", Prep},
		{"POC for new commerce API integration", "", Prep},
		{"Commerce | iOS | Architecture Review", "", Prep},
		{"Discovery — guest profile service gaps", "", Prep},
		{"Commerce | Add points display to dashboard", "", Feature},
		{"Fix NPE in CustomerProfileService", "", Bug},
		{"Refactor guest facade", "", Refactor},
		{"Regular story with no prep signals", "some description about feature preparation", Prep},
	}
	for _, tc := range cases {
		issue := &jira.Issue{Summary: tc.summary, Description: tc.desc, Type: "Story"}
		got := Classify(issue)
		if got != tc.want {
			t.Errorf("Classify(%q) = %s, want %s", tc.summary, got, tc.want)
		}
	}
}

func TestExtractJSONMarkdownFence(t *testing.T) {
	input := "```json\n{\"summary\": \"test\", \"steps\": []}\n```"
	result := extractJSON(input)
	if result != `{"summary": "test", "steps": []}` {
		t.Fatalf("unexpected: %s", result)
	}
}

func TestCleanJSONTrailingComma(t *testing.T) {
	input := `{"requirements": [{"id": "REQ-1",},]}`
	result := cleanJSON(input)
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("cleanJSON didn't fix trailing commas: %v\nresult: %s", err, result)
	}
}
