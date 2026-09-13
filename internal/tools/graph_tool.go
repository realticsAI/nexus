package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anurag/nexus/internal/analyzer"
	"github.com/anurag/nexus/internal/crawler"
	"github.com/anurag/nexus/internal/graph"
	"github.com/anurag/nexus/internal/mcp"
	"github.com/anurag/nexus/model"
)

type TraceDependenciesTool struct{ ctx *Context }

func NewTraceDependenciesTool(ctx *Context) *TraceDependenciesTool {
	return &TraceDependenciesTool{ctx: ctx}
}
func (t *TraceDependenciesTool) Name() string { return "trace_dependencies" }
func (t *TraceDependenciesTool) Description() string {
	return "Trace outbound dependencies of a service — what it calls via HTTP, Kafka, build deps."
}
func (t *TraceDependenciesTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service":   map[string]any{"type": "string", "description": "Service key"},
			"max_depth": map[string]any{"type": "number", "description": "Max traversal depth (default 3)"},
		},
		"required": []string{"service"},
	}
}
func (t *TraceDependenciesTool) Run(args map[string]any) *mcp.ToolCallResult {
	service, _ := args["service"].(string)
	depth := intArg(args, "max_depth", 3)

	edges := graph.TraceDependencies(t.ctx.Graph, service, graph.Outbound, depth)
	data, _ := json.MarshalIndent(map[string]any{
		"service": service,
		"depth":   depth,
		"count":   len(edges),
		"edges":   formatEdges(edges),
	}, "", "  ")
	return mcp.TextResult(string(data))
}

type TraceDependentsTool struct{ ctx *Context }

func NewTraceDependentsTool(ctx *Context) *TraceDependentsTool {
	return &TraceDependentsTool{ctx: ctx}
}
func (t *TraceDependentsTool) Name() string { return "trace_dependents" }
func (t *TraceDependentsTool) Description() string {
	return "Trace inbound dependents — what services call this one."
}
func (t *TraceDependentsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service":   map[string]any{"type": "string", "description": "Service key"},
			"max_depth": map[string]any{"type": "number", "description": "Max depth (default 3)"},
		},
		"required": []string{"service"},
	}
}
func (t *TraceDependentsTool) Run(args map[string]any) *mcp.ToolCallResult {
	service, _ := args["service"].(string)
	depth := intArg(args, "max_depth", 3)
	edges := graph.TraceDependencies(t.ctx.Graph, service, graph.Inbound, depth)
	data, _ := json.MarshalIndent(map[string]any{
		"service": service, "depth": depth, "count": len(edges), "edges": formatEdges(edges),
	}, "", "  ")
	return mcp.TextResult(string(data))
}

type ImpactAnalysisTool struct{ ctx *Context }

func NewImpactAnalysisTool(ctx *Context) *ImpactAnalysisTool {
	return &ImpactAnalysisTool{ctx: ctx}
}
func (t *ImpactAnalysisTool) Name() string { return "impact_analysis" }
func (t *ImpactAnalysisTool) Description() string {
	return "Analyze blast radius — which services would be affected by changes to this service."
}
func (t *ImpactAnalysisTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service":   map[string]any{"type": "string", "description": "Service key"},
			"max_depth": map[string]any{"type": "number", "description": "Max depth (default 3)"},
		},
		"required": []string{"service"},
	}
}
func (t *ImpactAnalysisTool) Run(args map[string]any) *mcp.ToolCallResult {
	service, _ := args["service"].(string)
	depth := intArg(args, "max_depth", 3)

	impact := graph.ImpactAnalysis(t.ctx.Graph, service, depth)
	data, _ := json.MarshalIndent(map[string]any{
		"service":           service,
		"affected_services": impact.AffectedServices,
		"depends_on":        impact.DependsOn,
		"affected_count":    len(impact.AffectedServices),
		"depends_on_count":  len(impact.DependsOn),
	}, "", "  ")
	return mcp.TextResult(string(data))
}

type StatusTool struct{ ctx *Context }

func NewStatusTool(ctx *Context) *StatusTool { return &StatusTool{ctx: ctx} }
func (t *StatusTool) Name() string           { return "status" }
func (t *StatusTool) Description() string    { return "Show index health: service count, edge count, last crawl stats." }
func (t *StatusTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *StatusTool) Run(args map[string]any) *mcp.ToolCallResult {
	h, _ := t.ctx.Store.LoadHealth()
	edgeTypes := make(map[string]int)
	for _, e := range t.ctx.Graph.Edges {
		edgeTypes[string(e.Type)]++
	}
	data, _ := json.MarshalIndent(map[string]any{
		"services":   len(t.ctx.Services),
		"nodes":      len(t.ctx.Graph.Nodes),
		"edges":      len(t.ctx.Graph.Edges),
		"edge_types": edgeTypes,
		"keywords":   len(t.ctx.Graph.KeywordIndex),
		"last_crawl": fmt.Sprintf("%d indexed, %d failed (%s)", h.Indexed, len(h.Failed), h.CrawlDuration),
	}, "", "  ")
	return mcp.TextResult(string(data))
}

type ReindexTool struct{ ctx *Context }

func NewReindexTool(ctx *Context) *ReindexTool { return &ReindexTool{ctx: ctx} }
func (t *ReindexTool) Name() string            { return "reindex" }
func (t *ReindexTool) Description() string {
	return "Re-crawl and re-index all repos, then hot-reload the in-memory index. Use after pulling new changes or when the index feels stale."
}
func (t *ReindexTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"full": map[string]any{"type": "boolean", "description": "Force full re-parse ignoring checksums (default false)"},
		},
	}
}
func (t *ReindexTool) Run(args map[string]any) *mcp.ToolCallResult {
	if t.ctx.Cfg == nil {
		return mcp.ErrorResult("reindex unavailable — config not set")
	}
	full, _ := args["full"].(bool)
	start := time.Now()

	c := crawler.NewCrawler(t.ctx.Cfg, t.ctx.Store)
	result, err := c.Run(full)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("crawl failed: %v", err))
	}

	services, err := t.ctx.Store.LoadAllServices()
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("load services failed: %v", err))
	}

	a := analyzer.NewAnalyzer(services)
	g := a.BuildGraph()
	if err := t.ctx.Store.SaveGraph(g); err != nil {
		return mcp.ErrorResult(fmt.Sprintf("save graph failed: %v", err))
	}

	t.ctx.SwapIndex(services, g)

	data, _ := json.MarshalIndent(map[string]any{
		"status":   "ok",
		"indexed":  result.Indexed,
		"failed":   len(result.Failed),
		"services": len(services),
		"edges":    len(g.Edges),
		"keywords": len(g.KeywordIndex),
		"duration": time.Since(start).Round(time.Millisecond).String(),
	}, "", "  ")
	return mcp.TextResult(string(data))
}

func formatEdges(edges []model.Edge) []map[string]string {
	result := make([]map[string]string, len(edges))
	for i, e := range edges {
		result[i] = map[string]string{
			"from": e.From, "to": e.To, "type": string(e.Type), "evidence": e.Evidence,
		}
	}
	return result
}
