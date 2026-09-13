package analyzer

import (
	"strings"

	"github.com/anurag/nexus/model"
)

type HttpEdgeResolver struct {
	registry *ServiceRegistry
}

func NewHttpEdgeResolver(r *ServiceRegistry) *HttpEdgeResolver {
	return &HttpEdgeResolver{registry: r}
}

func (h *HttpEdgeResolver) Resolve(services map[string]*model.ServiceIndex) []model.Edge {
	var edges []model.Edge

	for fromKey, svc := range services {
		for _, unit := range svc.Units {
			for _, call := range unit.ApiCalls {
				if target := h.resolveTarget(call, svc); target != "" && target != fromKey {
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       target,
						Type:     edgeTypeForTarget(target),
						Evidence: unit.Name + " → " + call,
					})
				}
			}
			for _, ref := range unit.ConfigRefs {
				url := h.registry.ResolveConfigURL(ref, svc)
				if url != "" {
					if target := h.resolveURLToService(url); target != "" && target != fromKey {
						edges = append(edges, model.Edge{
							From:     fromKey,
							To:       target,
							Type:     edgeTypeForTarget(target),
							Evidence: ref + " → " + url,
						})
					}
				}
			}
		}
		for configKey, configVal := range svc.Config {
			if isURLValue(configVal) {
				if target := h.resolveURLToService(configVal); target != "" && target != fromKey {
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       target,
						Type:     edgeTypeForTarget(target),
						Evidence: configKey + " = " + configVal,
					})
				}
			}
		}
	}
	return dedup(edges)
}

func edgeTypeForTarget(target string) model.EdgeType {
	if IsExternalNode(target) {
		return model.ExternalCall
	}
	return model.HTTPCall
}

func (h *HttpEdgeResolver) resolveTarget(call string, caller *model.ServiceIndex) string {
	if key, ok := h.registry.LookupEndpoint(call); ok {
		return key
	}
	return h.resolveURLToService(call)
}

func (h *HttpEdgeResolver) resolveURLToService(url string) string {
	name := extractHostServiceName(url)
	if name != "" {
		if key, ok := h.registry.LookupByName(name); ok {
			return key
		}
	}
	hostname := extractHostname(url)
	if hostname != "" {
		if sys := MatchHost(hostname); sys != nil {
			return ExternalNodeName(sys)
		}
	}
	return ""
}

func extractHostname(url string) string {
	s := strings.TrimPrefix(url, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "lb://")
	if idx := strings.IndexByte(s, '/'); idx > 0 {
		s = s[:idx]
	}
	if idx := strings.IndexByte(s, ':'); idx > 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func extractHostServiceName(url string) string {
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	if idx := strings.IndexByte(url, ':'); idx > 0 {
		url = url[:idx]
	}
	if idx := strings.IndexByte(url, '/'); idx > 0 {
		url = url[:idx]
	}
	parts := strings.Split(url, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return url
}

func isURLValue(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
