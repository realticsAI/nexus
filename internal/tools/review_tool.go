package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anurag/nexus/internal/graph"
	"github.com/anurag/nexus/internal/mcp"
)

type ReviewPRContextTool struct{ ctx *Context }

func NewReviewPRContextTool(ctx *Context) *ReviewPRContextTool {
	return &ReviewPRContextTool{ctx: ctx}
}
func (t *ReviewPRContextTool) Name() string { return "review_pr_context" }
func (t *ReviewPRContextTool) Description() string {
	return "Analyze the blast radius of a code change. Given file paths or service names, returns affected services, downstream consumers, Kafka topics, endpoints, and dependency chains to help review a PR."
}
func (t *ReviewPRContextTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"files":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Changed file paths"},
			"services": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Service keys directly involved"},
		},
	}
}
func (t *ReviewPRContextTool) Run(args map[string]any) *mcp.ToolCallResult {
	var affectedServices []string

	if svcs, ok := args["services"].([]any); ok {
		for _, s := range svcs {
			if str, ok := s.(string); ok {
				affectedServices = append(affectedServices, str)
			}
		}
	}

	if files, ok := args["files"].([]any); ok {
		for _, f := range files {
			filePath, ok := f.(string)
			if !ok {
				continue
			}
			for key, svc := range t.ctx.Services {
				for _, unit := range svc.Units {
					if strings.Contains(filePath, unit.File) || strings.HasSuffix(filePath, unit.File) {
						affectedServices = appendUnique(affectedServices, key)
					}
				}
			}
		}
	}

	if len(affectedServices) == 0 {
		return mcp.TextResult(`{"affected_services": [], "message": "No indexed services match the provided files/services"}`)
	}

	var allAffected []string
	var allEndpoints []string
	var allKafkaTopics []string
	edgesByType := make(map[string][]map[string]string)

	for _, svc := range affectedServices {
		impact := graph.ImpactAnalysis(t.ctx.Graph, svc, 3)
		allAffected = append(allAffected, impact.AffectedServices...)

		if s, ok := t.ctx.Services[svc]; ok {
			for _, unit := range s.Units {
				allEndpoints = append(allEndpoints, unit.Endpoints...)
				allKafkaTopics = append(allKafkaTopics, unit.KafkaProduces...)
				allKafkaTopics = append(allKafkaTopics, unit.KafkaConsumes...)
			}
		}

		for _, edge := range impact.InboundEdges {
			edgesByType[string(edge.Type)] = append(edgesByType[string(edge.Type)], map[string]string{
				"from": edge.From, "to": edge.To, "evidence": edge.Evidence,
			})
		}
		for _, edge := range impact.OutboundEdges {
			edgesByType[string(edge.Type)] = append(edgesByType[string(edge.Type)], map[string]string{
				"from": edge.From, "to": edge.To, "evidence": edge.Evidence,
			})
		}
	}

	risk := "LOW"
	if len(allAffected) > 5 {
		risk = "HIGH"
	} else if len(allAffected) > 2 {
		risk = "MEDIUM"
	}

	review := map[string]any{
		"directly_changed": affectedServices,
		"blast_radius":     dedupStrings(allAffected),
		"risk_level":       risk,
		"endpoints":        dedupStrings(allEndpoints),
		"kafka_topics":     dedupStrings(allKafkaTopics),
		"edges":            edgesByType,
		"review_notes":     buildReviewNotes(affectedServices, allAffected, risk),
	}

	data, _ := json.MarshalIndent(review, "", "  ")
	return mcp.TextResult(string(data))
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func dedupStrings(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func buildReviewNotes(changed, affected []string, risk string) []string {
	var notes []string
	if risk == "HIGH" {
		notes = append(notes, fmt.Sprintf("HIGH RISK: Changes affect %d downstream services — requires careful review", len(affected)))
	}
	if len(changed) > 1 {
		notes = append(notes, fmt.Sprintf("Cross-service change: %d services modified", len(changed)))
	}
	return notes
}
