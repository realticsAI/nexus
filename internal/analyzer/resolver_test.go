package analyzer

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func testServices() map[string]*model.ServiceIndex {
	return map[string]*model.ServiceIndex{
		"backend:orders": {
			Key: "backend:orders", Platform: model.Java,
			Units: []model.Unit{
				{Name: "OrderController", Endpoints: []string{"GET /api/orders", "POST /api/orders"},
					KafkaProduces: []string{"order-events"}},
			},
			Config:    map[string]string{"app.payment-url": "http://payment-svc:8080/api/pay"},
			BuildDeps: []string{"commons-lib"},
		},
		"backend:payments": {
			Key: "backend:payments", Platform: model.Java,
			Units: []model.Unit{
				{Name: "PaymentController", Endpoints: []string{"POST /api/pay"},
					KafkaConsumes: []string{"order-events"}},
			},
		},
		"backend:commons-lib": {
			Key: "backend:commons-lib", Platform: model.Java,
		},
		"web:shop": {
			Key: "web:shop", Platform: model.TypeScript,
			Units: []model.Unit{
				{Name: "OrderPage", ApiCalls: []string{"/api/orders"}},
			},
		},
	}
}

func TestKafkaEdgeResolver(t *testing.T) {
	svcs := testServices()
	resolver := NewKafkaEdgeResolver()
	edges := resolver.Resolve(svcs)
	if len(edges) != 1 {
		t.Fatalf("expected 1 kafka edge, got %d", len(edges))
	}
	if edges[0].From != "backend:orders" || edges[0].To != "backend:payments" {
		t.Fatalf("unexpected edge: %s → %s", edges[0].From, edges[0].To)
	}
	if edges[0].Type != model.KafkaProduce {
		t.Fatalf("expected KAFKA_PRODUCE, got %s", edges[0].Type)
	}
}

func TestBuildDepResolver(t *testing.T) {
	svcs := testServices()
	reg := NewRegistry(svcs)
	resolver := NewBuildDepResolver(reg)
	edges := resolver.Resolve(svcs)
	found := false
	for _, e := range edges {
		if e.From == "backend:orders" && e.To == "backend:commons-lib" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected build dep edge from orders to commons-lib")
	}
}

func TestApiCallResolver(t *testing.T) {
	svcs := testServices()
	reg := NewRegistry(svcs)
	resolver := NewApiCallResolver(reg)
	edges := resolver.Resolve(svcs)
	found := false
	for _, e := range edges {
		if e.From == "web:shop" && e.To == "backend:orders" && e.Type == model.FrontendAPICall {
			found = true
		}
	}
	if !found {
		t.Fatal("expected FRONTEND_API_CALL edge from shop to orders")
	}
}

func TestRegistryLookup(t *testing.T) {
	svcs := testServices()
	reg := NewRegistry(svcs)
	key, ok := reg.LookupByName("orders")
	if !ok {
		t.Fatal("should find orders by name")
	}
	if key != "backend:orders" {
		t.Fatalf("expected backend:orders, got %s", key)
	}

	nodes := reg.Nodes()
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
}

func TestDedup(t *testing.T) {
	edges := []model.Edge{
		{From: "a", To: "b", Type: model.HTTPCall},
		{From: "a", To: "b", Type: model.HTTPCall},
		{From: "a", To: "b", Type: model.KafkaProduce},
	}
	result := dedup(edges)
	if len(result) != 2 {
		t.Fatalf("expected 2 after dedup, got %d", len(result))
	}
}
