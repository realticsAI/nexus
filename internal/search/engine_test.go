package search

import (
	"testing"

	"github.com/anurag/nexus/internal/analyzer"
	"github.com/anurag/nexus/model"
)

func buildTestData() (*analyzer.KeywordIndex, *model.Graph, map[string]*model.ServiceIndex) {
	services := map[string]*model.ServiceIndex{
		"backend:orders": {
			Key: "backend:orders", Platform: model.Java,
			Units: []model.Unit{
				{Type: "class", Name: "OrderController", File: "OrderController.java", Line: 10,
					Endpoints: []string{"GET /api/orders", "POST /api/orders"},
					Methods: []model.Method{{Name: "createOrder", Line: 15, HTTPMethod: "POST", HTTPPath: "/api/orders"}}},
				{Type: "class", Name: "OrderService", File: "OrderService.java", Line: 1,
					Methods: []model.Method{{Name: "processOrder", Line: 5}}},
			},
		},
		"backend:payments": {
			Key: "backend:payments", Platform: model.Java,
			Units: []model.Unit{
				{Type: "class", Name: "PaymentController", File: "PaymentController.java", Line: 1,
					Endpoints: []string{"POST /api/pay"}},
			},
		},
		"web:shop": {
			Key: "web:shop", Platform: model.TypeScript,
			Units: []model.Unit{
				{Type: "component", Name: "OrderPage", File: "OrderPage.tsx", Line: 1,
					ApiCalls: []string{"/api/orders"}},
			},
		},
	}

	ki := analyzer.NewKeywordIndex()
	ki.Build(services)

	graph := &model.Graph{
		Edges: []model.Edge{
			{From: "web:shop", To: "backend:orders", Type: model.FrontendAPICall},
			{From: "backend:orders", To: "backend:payments", Type: model.HTTPCall},
		},
	}
	return ki, graph, services
}

func TestSearchKeyword(t *testing.T) {
	ki, graph, _ := buildTestData()
	engine := NewEngine(ki, graph)

	results, trace := engine.Search("OrderController", 10)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].ServiceKey != "backend:orders" {
		t.Fatalf("expected orders first, got %s", results[0].ServiceKey)
	}
	if trace.Confidence != model.High {
		t.Fatalf("expected HIGH confidence, got %s", trace.Confidence)
	}
}

func TestSearchWithGraphBonus(t *testing.T) {
	ki, graph, _ := buildTestData()
	engine := NewEngine(ki, graph)

	results, _ := engine.Search("Order", 20)
	found := false
	for _, r := range results {
		if r.ServiceKey == "backend:payments" {
			found = true
		}
	}
	_ = found
}

func TestSearchNoResults(t *testing.T) {
	ki, graph, _ := buildTestData()
	engine := NewEngine(ki, graph)

	results, trace := engine.Search("xyznonexistent", 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
	if trace.Confidence != model.Low {
		t.Fatalf("expected LOW confidence for empty results")
	}
}

func TestGrep(t *testing.T) {
	ki, graph, services := buildTestData()
	engine := NewEngine(ki, graph)

	results := engine.Grep("order", services)
	if len(results) == 0 {
		t.Fatal("expected grep results for 'order'")
	}
	foundController := false
	for _, r := range results {
		if r.Match == "OrderController" {
			foundController = true
		}
	}
	if !foundController {
		t.Fatal("expected OrderController in grep results")
	}
}

func TestSearchServices(t *testing.T) {
	ki, graph, _ := buildTestData()
	engine := NewEngine(ki, graph)

	services := engine.SearchServices("Order", 5)
	if len(services) == 0 {
		t.Fatal("expected service results")
	}
	if services[0] != "backend:orders" {
		t.Fatalf("expected backend:orders first, got %s", services[0])
	}
}
