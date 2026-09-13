package analyzer

import (
	"strings"
	"time"

	"github.com/anurag/nexus/model"
)

// Analyzer orchestrates all edge resolvers to build the full dependency graph.
type Analyzer struct {
	services map[string]*model.ServiceIndex
	registry *ServiceRegistry
}

// NewAnalyzer creates an Analyzer from the set of indexed services.
func NewAnalyzer(services map[string]*model.ServiceIndex) *Analyzer {
	reg := NewRegistry(services)
	return &Analyzer{
		services: services,
		registry: reg,
	}
}

// BuildGraph runs every resolver, builds the keyword index, and returns
// a deduplicated Graph ready for persistence.
func (a *Analyzer) BuildGraph() *model.Graph {
	var allEdges []model.Edge

	httpResolver := NewHttpEdgeResolver(a.registry)
	allEdges = append(allEdges, httpResolver.Resolve(a.services)...)

	kafkaResolver := NewKafkaEdgeResolver()
	allEdges = append(allEdges, kafkaResolver.Resolve(a.services)...)

	apiResolver := NewApiCallResolver(a.registry)
	allEdges = append(allEdges, apiResolver.Resolve(a.services)...)

	buildResolver := NewBuildDepResolver(a.registry)
	allEdges = append(allEdges, buildResolver.Resolve(a.services)...)

	configURLResolver := NewConfigURLResolver(a.registry)
	configURLEdges, configURLExtNodes := configURLResolver.Resolve(a.services)
	allEdges = append(allEdges, configURLEdges...)

	configEdges, externalNodes := a.resolveConfigAPIs()
	allEdges = append(allEdges, configEdges...)

	ki := NewKeywordIndex()
	ki.Build(a.services)

	nodes := a.registry.Nodes()
	nodes = append(nodes, configURLExtNodes...)
	nodes = append(nodes, externalNodes...)

	nodeSet := map[string]bool{}
	for _, n := range nodes {
		nodeSet[n.Key] = true
	}
	edges := dedup(allEdges)
	for _, e := range edges {
		if IsExternalNode(e.To) && !nodeSet[e.To] {
			nodeSet[e.To] = true
			label := strings.TrimPrefix(e.To, "external:")
			nodes = append(nodes, model.Node{
				Key:      e.To,
				Name:     label,
				Platform: "external",
			})
		}
	}

	return &model.Graph{
		Version:      1,
		Nodes:        nodes,
		Edges:        edges,
		KeywordIndex: ki.Index,
		GeneratedAt:  time.Now(),
	}
}

func (a *Analyzer) resolveConfigAPIs() ([]model.Edge, []model.Node) {
	var edges []model.Edge
	externalSet := map[string]bool{}
	var externalNodes []model.Node

	addExternal := func(key, label string) {
		if !externalSet[key] {
			externalSet[key] = true
			externalNodes = append(externalNodes, model.Node{
				Key:      key,
				Name:     label,
				Platform: "external",
			})
		}
	}

	for fromKey, svc := range a.services {
		for configKey, configVal := range svc.Config {
			if !strings.Contains(configKey, "domain") && !strings.Contains(configKey, "url") && !strings.Contains(configKey, "host") {
				continue
			}
			apiName := extractAPIName(configVal)
			if apiName == "" {
				continue
			}

			if target, ok := a.registry.LookupByName(apiName); ok && target != fromKey {
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       target,
					Type:     model.HTTPCall,
					Evidence: "config: " + configKey,
				})
				continue
			}
			if target, ok := a.registry.LookupByName(strings.ReplaceAll(apiName, "-", "_")); ok && target != fromKey {
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       target,
					Type:     model.HTTPCall,
					Evidence: "config: " + configKey,
				})
				continue
			}

			if sys := MatchHost(apiName); sys != nil {
				extKey := ExternalNodeName(sys)
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       extKey,
					Type:     model.ExternalCall,
					Evidence: "config: " + configKey,
				})
				addExternal(extKey, sys.Label)
				continue
			}
			if sys := MatchConfigRef(configKey); sys != nil {
				extKey := ExternalNodeName(sys)
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       extKey,
					Type:     model.ExternalCall,
					Evidence: "config: " + configKey,
				})
				addExternal(extKey, sys.Label)
				continue
			}

			extKey := "external:" + apiName
			edges = append(edges, model.Edge{
				From:     fromKey,
				To:       extKey,
				Type:     model.HTTPCall,
				Evidence: "config: " + configKey,
			})
			addExternal(extKey, apiName)
		}
	}
	return edges, externalNodes
}

func extractAPIName(val string) string {
	if idx := strings.LastIndex(val, "/"); idx > 0 {
		segment := val[strings.LastIndex(val[:idx], "/")+1 : idx]
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "v1" || segment == "v2" || segment == "v3" || strings.HasPrefix(segment, "$") || strings.HasPrefix(segment, "http") {
			return ""
		}
		if strings.Contains(segment, "}") {
			after := strings.Index(segment, "}")
			if after+1 < len(segment) {
				segment = segment[after+1:]
				segment = strings.TrimPrefix(segment, "/")
				if slashIdx := strings.Index(segment, "/"); slashIdx > 0 {
					segment = segment[:slashIdx]
				}
			} else {
				return ""
			}
		}
		if len(segment) > 2 && !strings.Contains(segment, " ") {
			skip := []string{"api-docs", "swagger", "actuator", "internal", "health", "info", "metrics"}
			for _, s := range skip {
				if segment == s {
					return ""
				}
			}
			return segment
		}
	}
	return ""
}
