package graph

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func testGraph() *model.Graph {
	return &model.Graph{
		Version: 1,
		Nodes: []model.Node{
			{Key: "web:shop"}, {Key: "backend:orders"}, {Key: "backend:payments"}, {Key: "backend:inventory"},
		},
		Edges: []model.Edge{
			{From: "web:shop", To: "backend:orders", Type: model.FrontendAPICall},
			{From: "backend:orders", To: "backend:payments", Type: model.HTTPCall},
			{From: "backend:orders", To: "backend:inventory", Type: model.KafkaProduce},
			{From: "backend:payments", To: "backend:inventory", Type: model.HTTPCall},
		},
	}
}

func TestTraceDependenciesOutbound(t *testing.T) {
	g := testGraph()
	edges := TraceDependencies(g, "backend:orders", Outbound, 2)
	if len(edges) < 2 {
		t.Fatalf("expected at least 2 outbound edges from orders, got %d", len(edges))
	}
}

func TestTraceDependenciesInbound(t *testing.T) {
	g := testGraph()
	edges := TraceDependencies(g, "backend:orders", Inbound, 2)
	if len(edges) == 0 {
		t.Fatal("expected inbound edges to orders (from web:shop)")
	}
	found := false
	for _, e := range edges {
		if e.From == "web:shop" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected web:shop in inbound edges")
	}
}

func TestImpactAnalysis(t *testing.T) {
	g := testGraph()
	impact := ImpactAnalysis(g, "backend:orders", 3)
	if impact.ServiceKey != "backend:orders" {
		t.Fatalf("wrong service key")
	}
	if len(impact.AffectedServices) == 0 {
		t.Fatal("expected affected services")
	}
	if len(impact.DependsOn) == 0 {
		t.Fatal("expected depends-on services")
	}
}

func TestNeighbors(t *testing.T) {
	g := testGraph()
	edges := Neighbors(g, "backend:orders")
	if len(edges) < 3 {
		t.Fatalf("expected at least 3 neighbor edges, got %d", len(edges))
	}
}

func TestFilterEdgesByType(t *testing.T) {
	g := testGraph()
	http := FilterEdgesByType(g.Edges, model.HTTPCall)
	if len(http) != 2 {
		t.Fatalf("expected 2 HTTP_CALL edges, got %d", len(http))
	}
	kafka := FilterEdgesByType(g.Edges, model.KafkaProduce)
	if len(kafka) != 1 {
		t.Fatalf("expected 1 KAFKA_PRODUCE edge, got %d", len(kafka))
	}
}

func TestMaxDepthLimit(t *testing.T) {
	g := testGraph()
	depth1 := TraceDependencies(g, "web:shop", Outbound, 1)
	depthAll := TraceDependencies(g, "web:shop", Outbound, 10)
	if len(depth1) >= len(depthAll) && len(depthAll) > 1 {
		t.Fatal("deeper search should find more or equal edges")
	}
}
