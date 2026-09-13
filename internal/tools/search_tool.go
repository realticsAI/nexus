package tools

import (
	"encoding/json"
	"strings"

	"github.com/anurag/nexus/internal/mcp"
	"github.com/anurag/nexus/model"
)

type SearchTool struct {
	ctx *Context
}

func NewSearchTool(ctx *Context) *SearchTool { return &SearchTool{ctx: ctx} }
func (t *SearchTool) Name() string           { return "search" }
func (t *SearchTool) Description() string {
	return "Search the codebase index using keyword matching. Returns classes, methods, endpoints, and services matching the query."
}
func (t *SearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":   map[string]any{"type": "string", "description": "Search query (class name, method, endpoint, topic)"},
			"limit":   map[string]any{"type": "number", "description": "Max results (default 20)"},
			"service": map[string]any{"type": "string", "description": "Filter to specific service key"},
		},
		"required": []string{"query"},
	}
}
func (t *SearchTool) Run(args map[string]any) *mcp.ToolCallResult {
	query, _ := args["query"].(string)
	if query == "" {
		return mcp.ErrorResult("query is required")
	}
	limit := intArg(args, "limit", 20)
	serviceFilter, _ := args["service"].(string)

	var results []model.Hit
	for token, hits := range t.ctx.Graph.KeywordIndex {
		if !strings.Contains(strings.ToLower(token), strings.ToLower(query)) {
			continue
		}
		for _, h := range hits {
			if serviceFilter != "" && h.ServiceKey != serviceFilter {
				continue
			}
			results = append(results, h)
		}
	}
	sortHits(results)
	if len(results) > limit {
		results = results[:limit]
	}

	data, _ := json.MarshalIndent(map[string]any{
		"query":   query,
		"count":   len(results),
		"results": results,
	}, "", "  ")
	return mcp.TextResult(string(data))
}

type GrepTool struct {
	ctx *Context
}

func NewGrepTool(ctx *Context) *GrepTool { return &GrepTool{ctx: ctx} }
func (t *GrepTool) Name() string         { return "grep" }
func (t *GrepTool) Description() string {
	return "Search for a pattern across all indexed services. Matches against class names, method names, endpoints, and annotations."
}
func (t *GrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Pattern to search for"},
			"service": map[string]any{"type": "string", "description": "Filter to specific service"},
		},
		"required": []string{"pattern"},
	}
}
func (t *GrepTool) Run(args map[string]any) *mcp.ToolCallResult {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return mcp.ErrorResult("pattern is required")
	}
	serviceFilter, _ := args["service"].(string)
	lowerPattern := strings.ToLower(pattern)

	type match struct {
		Service string `json:"service"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		File    string `json:"file"`
		Line    int    `json:"line"`
	}
	var matches []match

	for key, svc := range t.ctx.Services {
		if serviceFilter != "" && key != serviceFilter {
			continue
		}
		for _, unit := range svc.Units {
			if strings.Contains(strings.ToLower(unit.Name), lowerPattern) {
				matches = append(matches, match{Service: key, Type: unit.Type, Name: unit.Name, File: unit.File, Line: unit.Line})
			}
			for _, m := range unit.Methods {
				if strings.Contains(strings.ToLower(m.Name), lowerPattern) {
					matches = append(matches, match{Service: key, Type: "method", Name: unit.Name + "." + m.Name, File: unit.File, Line: m.Line})
				}
			}
			for _, ep := range unit.Endpoints {
				if strings.Contains(strings.ToLower(ep), lowerPattern) {
					matches = append(matches, match{Service: key, Type: "endpoint", Name: ep, File: unit.File, Line: unit.Line})
				}
			}
		}
	}
	if len(matches) > 50 {
		matches = matches[:50]
	}

	data, _ := json.MarshalIndent(map[string]any{"pattern": pattern, "count": len(matches), "matches": matches}, "", "  ")
	return mcp.TextResult(string(data))
}

type FindEndpointsTool struct {
	ctx *Context
}

func NewFindEndpointsTool(ctx *Context) *FindEndpointsTool { return &FindEndpointsTool{ctx: ctx} }
func (t *FindEndpointsTool) Name() string                  { return "find_endpoints" }
func (t *FindEndpointsTool) Description() string {
	return "Find HTTP endpoints across all services. Optionally filter by path pattern or HTTP method."
}
func (t *FindEndpointsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path pattern to match (e.g. /api/orders)"},
			"method":  map[string]any{"type": "string", "description": "HTTP method filter (GET, POST, etc.)"},
			"service": map[string]any{"type": "string", "description": "Filter to specific service"},
		},
	}
}
func (t *FindEndpointsTool) Run(args map[string]any) *mcp.ToolCallResult {
	pathFilter, _ := args["path"].(string)
	methodFilter, _ := args["method"].(string)
	serviceFilter, _ := args["service"].(string)

	type endpoint struct {
		Service string `json:"service"`
		Method  string `json:"method"`
		Path    string `json:"path"`
		Handler string `json:"handler"`
		File    string `json:"file"`
		Line    int    `json:"line"`
	}
	var endpoints []endpoint

	for key, svc := range t.ctx.Services {
		if serviceFilter != "" && key != serviceFilter {
			continue
		}
		for _, unit := range svc.Units {
			for _, m := range unit.Methods {
				if m.HTTPMethod == "" {
					continue
				}
				if methodFilter != "" && !strings.EqualFold(m.HTTPMethod, methodFilter) {
					continue
				}
				if pathFilter != "" && !strings.Contains(m.HTTPPath, pathFilter) {
					continue
				}
				endpoints = append(endpoints, endpoint{
					Service: key, Method: m.HTTPMethod, Path: m.HTTPPath,
					Handler: unit.Name + "." + m.Name, File: unit.File, Line: m.Line,
				})
			}
		}
	}

	data, _ := json.MarshalIndent(map[string]any{"count": len(endpoints), "endpoints": endpoints}, "", "  ")
	return mcp.TextResult(string(data))
}

func intArg(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

func sortHits(hits []model.Hit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}
