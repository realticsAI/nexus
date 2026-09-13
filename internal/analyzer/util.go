package analyzer

import "github.com/anurag/nexus/model"

func dedup(edges []model.Edge) []model.Edge {
	seen := make(map[string]bool)
	var result []model.Edge
	for _, e := range edges {
		key := e.From + "|" + e.To + "|" + string(e.Type)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}
