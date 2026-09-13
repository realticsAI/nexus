package analyzer

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func TestKeywordIndexBuild(t *testing.T) {
	ki := NewKeywordIndex()
	services := map[string]*model.ServiceIndex{
		"backend:orders": {
			Key: "backend:orders", Platform: model.Java,
			Units: []model.Unit{
				{Type: "class", Name: "OrderController", Stereotype: "rest_controller",
					Methods: []model.Method{{Name: "createOrder", HTTPMethod: "POST", HTTPPath: "/api/orders"}},
					Endpoints: []string{"POST /api/orders"},
					KafkaProduces: []string{"order-events"}},
			},
		},
	}
	ki.Build(services)
	if len(ki.Index) == 0 {
		t.Fatal("index should not be empty")
	}
	if _, ok := ki.Index["OrderController"]; !ok {
		t.Fatal("should index class name")
	}
	if _, ok := ki.Index["order-events"]; !ok {
		t.Fatal("should index kafka topic")
	}
}

func TestKeywordSearch(t *testing.T) {
	ki := NewKeywordIndex()
	ki.Build(map[string]*model.ServiceIndex{
		"backend:orders": {
			Key: "backend:orders", Platform: model.Java,
			Units: []model.Unit{
				{Type: "class", Name: "OrderController",
					Methods: []model.Method{{Name: "createOrder"}}},
				{Type: "class", Name: "OrderService",
					Methods: []model.Method{{Name: "processOrder"}}},
			},
		},
		"backend:payments": {
			Key: "backend:payments", Platform: model.Java,
			Units: []model.Unit{
				{Type: "class", Name: "PaymentController"},
			},
		},
	})

	results := ki.Search("Order", 10)
	if len(results) == 0 {
		t.Fatal("expected results for 'Order'")
	}
	if results[0].ServiceKey != "backend:orders" {
		t.Fatalf("orders should rank first, got %s", results[0].ServiceKey)
	}
}

func TestSplitCamelCase(t *testing.T) {
	cases := map[string]int{
		"OrderController": 2,
		"createOrder":     2,
		"order-service":   2,
		"commerce.cart":   2,
		"simple":          1,
	}
	for input, wantLen := range cases {
		parts := splitCamelCase(input)
		if len(parts) != wantLen {
			t.Errorf("splitCamelCase(%q) = %v (len %d), want len %d", input, parts, len(parts), wantLen)
		}
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("OrderController createOrder")
	if len(tokens) < 4 {
		t.Fatalf("expected at least 4 tokens (words + camelCase splits), got %d: %v", len(tokens), tokens)
	}
}

func TestSearchLimit(t *testing.T) {
	ki := NewKeywordIndex()
	for i := 0; i < 100; i++ {
		ki.addToken("common", model.Hit{ServiceKey: "svc-" + string(rune('a'+i%26)), Score: 1})
	}
	results := ki.Search("common", 5)
	if len(results) > 5 {
		t.Fatalf("expected at most 5 results, got %d", len(results))
	}
}
