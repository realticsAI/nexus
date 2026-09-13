package graph

import "github.com/anurag/nexus/model"

// Direction controls which edges BFS follows.
type Direction int

const (
	Outbound Direction = iota // follow From→To
	Inbound                   // follow To→From
	Both                      // follow both directions
)

// TraceDependencies performs BFS from serviceKey, collecting edges up to
// maxDepth hops away in the given Direction.
func TraceDependencies(g *model.Graph, serviceKey string, direction Direction, maxDepth int) []model.Edge {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	adjacency := buildAdjacency(g.Edges, direction)

	visited := make(map[string]bool)
	var result []model.Edge
	queue := []struct {
		key   string
		depth int
	}{{serviceKey, 0}}
	visited[serviceKey] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth >= maxDepth {
			continue
		}

		for _, edge := range adjacency[current.key] {
			result = append(result, edge)
			next := edge.To
			if direction == Inbound {
				next = edge.From
			}
			if !visited[next] {
				visited[next] = true
				queue = append(queue, struct {
					key   string
					depth int
				}{next, current.depth + 1})
			}
		}
	}
	return result
}

// ImpactResult holds the full impact picture for a single service.
type ImpactResult struct {
	ServiceKey       string       `json:"service_key"`
	AffectedServices []string     `json:"affected_services"`
	DependsOn        []string     `json:"depends_on"`
	InboundEdges     []model.Edge `json:"inbound_edges"`
	OutboundEdges    []model.Edge `json:"outbound_edges"`
}

// ImpactAnalysis computes which services are affected by changes to
// serviceKey (inbound callers) and which it depends on (outbound targets).
func ImpactAnalysis(g *model.Graph, serviceKey string, maxDepth int) *ImpactResult {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	outbound := TraceDependencies(g, serviceKey, Outbound, maxDepth)
	inbound := TraceDependencies(g, serviceKey, Inbound, maxDepth)

	affected := make(map[string]bool)
	for _, e := range inbound {
		if e.From != serviceKey {
			affected[e.From] = true
		}
	}

	dependsOn := make(map[string]bool)
	for _, e := range outbound {
		if e.To != serviceKey {
			dependsOn[e.To] = true
		}
	}

	return &ImpactResult{
		ServiceKey:       serviceKey,
		AffectedServices: mapKeys(affected),
		DependsOn:        mapKeys(dependsOn),
		InboundEdges:     inbound,
		OutboundEdges:    outbound,
	}
}

// Neighbors returns every edge that touches serviceKey (from or to).
func Neighbors(g *model.Graph, serviceKey string) []model.Edge {
	var result []model.Edge
	for _, e := range g.Edges {
		if e.From == serviceKey || e.To == serviceKey {
			result = append(result, e)
		}
	}
	return result
}

// FilterEdgesByType keeps only the edges that match edgeType.
func FilterEdgesByType(edges []model.Edge, edgeType model.EdgeType) []model.Edge {
	var result []model.Edge
	for _, e := range edges {
		if e.Type == edgeType {
			result = append(result, e)
		}
	}
	return result
}

// buildAdjacency indexes edges by the node that is the "source" according
// to direction so that BFS can look them up in O(1).
func buildAdjacency(edges []model.Edge, direction Direction) map[string][]model.Edge {
	adj := make(map[string][]model.Edge)
	for _, e := range edges {
		switch direction {
		case Outbound:
			adj[e.From] = append(adj[e.From], e)
		case Inbound:
			adj[e.To] = append(adj[e.To], e)
		case Both:
			adj[e.From] = append(adj[e.From], e)
			adj[e.To] = append(adj[e.To], e)
		}
	}
	return adj
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
