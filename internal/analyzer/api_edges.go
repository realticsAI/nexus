package analyzer

import (
	"strings"

	"github.com/anurag/nexus/model"
)

type ApiCallResolver struct {
	registry *ServiceRegistry
}

func NewApiCallResolver(r *ServiceRegistry) *ApiCallResolver {
	return &ApiCallResolver{registry: r}
}

func (a *ApiCallResolver) Resolve(services map[string]*model.ServiceIndex) []model.Edge {
	var edges []model.Edge

	for fromKey, svc := range services {
		for _, unit := range svc.Units {
			for _, call := range unit.ApiCalls {
				path := normalizeAPIPath(call)
				if target, ok := a.registry.LookupEndpoint(path); ok && target != fromKey {
					edgeType := model.FrontendAPICall
					if svc.Platform == model.Java {
						edgeType = model.HTTPCall
					}
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       target,
						Type:     edgeType,
						Evidence: unit.Name + " → " + call,
					})
				}
			}
		}

		for configKey, configVal := range svc.Config {
			urlPath := extractURLPath(configVal)
			if urlPath == "" {
				continue
			}
			if target, ok := a.registry.LookupEndpoint(urlPath); ok && target != fromKey {
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       target,
					Type:     model.HTTPCall,
					Evidence: "config: " + configKey + " → " + urlPath,
				})
			}
			if target, ok := a.registry.LookupByName(extractServiceName(urlPath)); ok && target != fromKey {
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       target,
					Type:     model.HTTPCall,
					Evidence: "config: " + configKey + " → " + urlPath,
				})
			}
		}
	}
	return dedup(edges)
}

func extractURLPath(val string) string {
	if idx := strings.Index(val, "/"); idx >= 0 {
		path := val[idx:]
		if strings.Contains(path, "${") {
			if end := strings.Index(path, "}"); end >= 0 {
				path = path[end+1:]
			}
		}
		if colon := strings.Index(path, ":"); colon > 0 {
			path = path[:colon]
		}
		path = strings.TrimRight(path, "/}")
		if len(path) > 1 {
			return path
		}
	}
	return ""
}

func extractServiceName(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 0 {
		name := strings.ReplaceAll(parts[0], "-", "_")
		return name
	}
	return ""
}

func normalizeAPIPath(call string) string {
	call = strings.TrimPrefix(call, "http://localhost:8080")
	call = strings.TrimPrefix(call, "http://localhost:3000")
	if idx := strings.IndexByte(call, '?'); idx > 0 {
		call = call[:idx]
	}
	return call
}
