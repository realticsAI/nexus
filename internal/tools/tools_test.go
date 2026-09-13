package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
)

func testContext() *Context {
	s := store.NewStore("/tmp/nexus-test-tools")
	services := map[string]*model.ServiceIndex{
		"backend:orders": {
			Key: "backend:orders", Platform: model.Java, Path: "/tmp/orders",
			Units: []model.Unit{
				{Type: "class", Name: "OrderController", File: "OrderController.java", Line: 10,
					Endpoints: []string{"GET /api/orders", "POST /api/orders"},
					Methods:   []model.Method{{Name: "createOrder", HTTPMethod: "POST", HTTPPath: "/api/orders", Line: 15}}},
			},
			FileChecksums: map[string]string{"OrderController.java": "sha256:abc"},
			BuildDeps:     []string{"commons-lib"},
		},
		"backend:payments": {
			Key: "backend:payments", Platform: model.Java, Path: "/tmp/payments",
			Units: []model.Unit{
				{Type: "class", Name: "PaymentService", File: "PaymentService.java", Line: 1},
			},
			FileChecksums: map[string]string{},
		},
	}
	graph := &model.Graph{
		Nodes: []model.Node{{Key: "backend:orders"}, {Key: "backend:payments"}},
		Edges: []model.Edge{{From: "backend:orders", To: "backend:payments", Type: model.HTTPCall}},
		KeywordIndex: map[string][]model.Hit{
			"OrderController": {{ServiceKey: "backend:orders", UnitType: "class", UnitName: "OrderController", Score: 10}},
			"orders":          {{ServiceKey: "backend:orders", UnitType: "service", UnitName: "orders", Score: 15}},
		},
	}
	return &Context{Store: s, Services: services, Graph: graph}
}

func TestSearchTool(t *testing.T) {
	ctx := testContext()
	tool := NewSearchTool(ctx)
	if tool.Name() != "search" {
		t.Fatal("wrong name")
	}
	result := tool.Run(map[string]any{"query": "Order"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "OrderController") {
		t.Fatal("expected OrderController in results")
	}
}

func TestListServicesTool(t *testing.T) {
	ctx := testContext()
	tool := NewListServicesTool(ctx)
	result := tool.Run(map[string]any{})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	var out map[string]any
	json.Unmarshal([]byte(result.Content[0].Text), &out)
	count := int(out["count"].(float64))
	if count != 2 {
		t.Fatalf("expected 2 services, got %d", count)
	}
}

func TestServiceProfileTool(t *testing.T) {
	ctx := testContext()
	tool := NewServiceProfileTool(ctx)
	result := tool.Run(map[string]any{"service": "backend:orders"})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !strings.Contains(result.Content[0].Text, "orders") {
		t.Fatal("expected orders in profile")
	}
}

func TestServiceProfileNotFound(t *testing.T) {
	ctx := testContext()
	tool := NewServiceProfileTool(ctx)
	result := tool.Run(map[string]any{"service": "nonexistent"})
	if !result.IsError {
		t.Fatal("expected error for unknown service")
	}
}

func TestFindEndpointsTool(t *testing.T) {
	ctx := testContext()
	tool := NewFindEndpointsTool(ctx)
	result := tool.Run(map[string]any{"path": "/api/orders"})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !strings.Contains(result.Content[0].Text, "/api/orders") {
		t.Fatal("expected /api/orders endpoint")
	}
}

func TestTraceDependenciesTool(t *testing.T) {
	ctx := testContext()
	tool := NewTraceDependenciesTool(ctx)
	result := tool.Run(map[string]any{"service": "backend:orders"})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !strings.Contains(result.Content[0].Text, "backend:payments") {
		t.Fatal("expected payments in dependencies")
	}
}

func TestImpactAnalysisTool(t *testing.T) {
	ctx := testContext()
	tool := NewImpactAnalysisTool(ctx)
	result := tool.Run(map[string]any{"service": "backend:payments"})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !strings.Contains(result.Content[0].Text, "backend:orders") {
		t.Fatal("expected orders in affected services")
	}
}

func TestGrepTool(t *testing.T) {
	ctx := testContext()
	tool := NewGrepTool(ctx)
	result := tool.Run(map[string]any{"pattern": "order"})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !strings.Contains(result.Content[0].Text, "OrderController") {
		t.Fatal("expected OrderController in grep results")
	}
}

func TestStatusTool(t *testing.T) {
	ctx := testContext()
	tool := NewStatusTool(ctx)
	result := tool.Run(map[string]any{})
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !strings.Contains(result.Content[0].Text, "services") {
		t.Fatal("expected services in status")
	}
}
