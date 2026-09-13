package search

import (
	"strings"
	"time"

	"github.com/anurag/nexus/internal/analyzer"
	"github.com/anurag/nexus/model"
)

// Result represents a single search hit with a merged score from multiple sources.
type Result struct {
	ServiceKey string  `json:"service_key"`
	UnitType   string  `json:"unit_type"`
	UnitName   string  `json:"unit_name"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
}

// Engine is the hybrid search engine that combines keyword search, semantic
// search (embeddings), and graph BFS bonus scoring.
type Engine struct {
	keywords *analyzer.KeywordIndex
	graph    *model.Graph
	semantic SemanticSearcher
}

// SemanticSearcher is an optional embedding-based search backend.
type SemanticSearcher interface {
	Search(query string, candidateServices []string, limit int) []Result
	Available() bool
}

// NewEngine creates a search engine backed by a keyword index and graph.
func NewEngine(ki *analyzer.KeywordIndex, graph *model.Graph) *Engine {
	return &Engine{
		keywords: ki,
		graph:    graph,
	}
}

// SetSemantic attaches an optional semantic search backend.
func (e *Engine) SetSemantic(s SemanticSearcher) {
	e.semantic = s
}

// Search runs the hybrid query: keyword lookup, optional semantic search,
// and graph-neighbor bonus scoring. It returns ranked results and a trace.
func (e *Engine) Search(query string, limit int) ([]Result, *model.TraceContext) {
	if limit <= 0 {
		limit = 20
	}
	start := time.Now()

	// --- Phase 1: keyword search ---
	keywordHits := e.keywords.Search(query, limit*3)
	results := make(map[string]*Result)
	var servicesConsulted []string

	for _, h := range keywordHits {
		key := h.ServiceKey + "|" + h.UnitName
		results[key] = &Result{
			ServiceKey: h.ServiceKey,
			UnitType:   h.UnitType,
			UnitName:   h.UnitName,
			Score:      float64(h.Score),
			Source:     "keyword",
		}
		servicesConsulted = appendUnique(servicesConsulted, h.ServiceKey)
	}

	// --- Phase 2: semantic search (if available) ---
	if e.semantic != nil && e.semantic.Available() {
		candidateServices := make([]string, 0, len(servicesConsulted))
		for _, s := range servicesConsulted {
			candidateServices = append(candidateServices, s)
		}
		// Only constrain to candidate services when there are enough to be meaningful.
		if len(candidateServices) < 30 {
			candidateServices = nil
		}
		semanticResults := e.semantic.Search(query, candidateServices, limit)
		for _, sr := range semanticResults {
			key := sr.ServiceKey + "|" + sr.UnitName
			if existing, ok := results[key]; ok {
				existing.Score += sr.Score
				existing.Source = "keyword+semantic"
			} else {
				sr.Source = "semantic"
				results[key] = &sr
				servicesConsulted = appendUnique(servicesConsulted, sr.ServiceKey)
			}
		}
	}

	// --- Phase 3: graph BFS bonus ---
	if e.graph != nil {
		topServices := topNServices(keywordHits, 5)
		for _, svcKey := range topServices {
			neighbors := graphNeighborKeys(e.graph, svcKey)
			for _, nKey := range neighbors {
				for k, r := range results {
					if r.ServiceKey == nKey {
						results[k].Score += 3
						if results[k].Source == "keyword" {
							results[k].Source = "keyword+graph"
						}
					}
				}
			}
		}
	}

	sorted := sortResults(results, limit)

	// --- Build trace context ---
	dataSources := []string{"keyword_index"}
	if e.semantic != nil && e.semantic.Available() {
		dataSources = append(dataSources, "embeddings")
	}
	if e.graph != nil {
		dataSources = append(dataSources, "graph")
	}

	confidence := model.High
	if len(sorted) == 0 {
		confidence = model.Low
	} else if sorted[0].Score < 10 {
		confidence = model.Medium
	}

	trace := &model.TraceContext{
		Confidence:       confidence,
		ServicesConsulted: servicesConsulted,
		DataSources:      dataSources,
		IndexAge:         time.Since(start).Round(time.Millisecond).String(),
		IndexTimestamp:   time.Now(),
	}

	return sorted, trace
}

// SearchServices is a convenience wrapper that returns only distinct service
// keys from the search results.
func (e *Engine) SearchServices(query string, limit int) []string {
	results, _ := e.Search(query, limit)
	seen := make(map[string]bool)
	var services []string
	for _, r := range results {
		if !seen[r.ServiceKey] {
			seen[r.ServiceKey] = true
			services = append(services, r.ServiceKey)
		}
	}
	return services
}

// GrepResult represents a literal pattern match inside indexed service metadata.
type GrepResult struct {
	ServiceKey string `json:"service_key"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Match      string `json:"match"`
	Context    string `json:"context"`
}

// Grep performs a case-insensitive literal search across all indexed units,
// methods, and endpoints.
func (e *Engine) Grep(pattern string, services map[string]*model.ServiceIndex) []GrepResult {
	var results []GrepResult
	pattern = strings.ToLower(pattern)
	for key, svc := range services {
		for _, unit := range svc.Units {
			if containsIgnoreCase(unit.Name, pattern) {
				results = append(results, GrepResult{ServiceKey: key, File: unit.File, Line: unit.Line, Match: unit.Name, Context: unit.Type + ": " + unit.Name})
			}
			for _, m := range unit.Methods {
				if containsIgnoreCase(m.Name, pattern) {
					results = append(results, GrepResult{ServiceKey: key, File: unit.File, Line: m.Line, Match: m.Name, Context: unit.Name + "." + m.Name})
				}
			}
			for _, ep := range unit.Endpoints {
				if containsIgnoreCase(ep, pattern) {
					results = append(results, GrepResult{ServiceKey: key, File: unit.File, Line: unit.Line, Match: ep, Context: "endpoint: " + ep})
				}
			}
		}
	}
	return results
}

// ---------- helpers ----------

func topNServices(hits []model.Hit, n int) []string {
	seen := make(map[string]bool)
	var result []string
	for _, h := range hits {
		if !seen[h.ServiceKey] {
			seen[h.ServiceKey] = true
			result = append(result, h.ServiceKey)
			if len(result) >= n {
				break
			}
		}
	}
	return result
}

func graphNeighborKeys(g *model.Graph, serviceKey string) []string {
	var keys []string
	for _, e := range g.Edges {
		if e.From == serviceKey && e.To != serviceKey {
			keys = append(keys, e.To)
		}
		if e.To == serviceKey && e.From != serviceKey {
			keys = append(keys, e.From)
		}
	}
	return keys
}

func sortResults(m map[string]*Result, limit int) []Result {
	results := make([]Result, 0, len(m))
	for _, r := range m {
		results = append(results, *r)
	}
	// Insertion sort -- adequate for the result-set sizes we deal with.
	// Break ties by service key then unit name for deterministic ordering.
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && resultLess(results[j-1], results[j]); j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// resultLess returns true when a should sort AFTER b (i.e., b ranks higher).
// Primary: higher score wins. Tiebreaker: lexicographic service key, then unit name.
func resultLess(a, b Result) bool {
	if a.Score != b.Score {
		return a.Score < b.Score
	}
	if a.ServiceKey != b.ServiceKey {
		return a.ServiceKey > b.ServiceKey
	}
	return a.UnitName > b.UnitName
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), substr)
}
