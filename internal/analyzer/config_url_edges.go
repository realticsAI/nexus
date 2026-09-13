package analyzer

import (
	"strings"

	"github.com/anurag/nexus/internal/crawler"
	"github.com/anurag/nexus/model"
)

// ConfigURLResolver finds edges by reading application.yml/properties
// config values, resolving ${placeholder} chains, and matching resolved
// URLs to known services by hostname. This catches the dominant pattern
// in Spring Boot codebases where inter-service calls route through
// config properties like app.calendar.url=http://calendar-svc:8080.
type ConfigURLResolver struct {
	registry *ServiceRegistry
}

func NewConfigURLResolver(r *ServiceRegistry) *ConfigURLResolver {
	return &ConfigURLResolver{registry: r}
}

func (c *ConfigURLResolver) Resolve(services map[string]*model.ServiceIndex) ([]model.Edge, []model.Node) {
	var edges []model.Edge
	seen := map[string]bool{}
	externalSet := map[string]bool{}
	var externalNodes []model.Node

	for fromKey, svc := range services {
		urls := crawler.ExtractServiceURLs(svc.Config)
		for _, cu := range urls {
			hostname := configExtractHost(cu.URL)
			if hostname == "" {
				continue
			}

			if target, ok := c.registry.LookupByName(hostname); ok && target != fromKey {
				edgeKey := fromKey + "|" + target
				if seen[edgeKey] {
					continue
				}
				seen[edgeKey] = true
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       target,
					Type:     model.HTTPCall,
					Evidence: cu.Key + " = " + cu.URL,
				})
				continue
			}

			stripped := hostname
			if strings.HasSuffix(stripped, "-service") {
				stripped = strings.TrimSuffix(stripped, "-service")
			}
			if stripped != hostname {
				if target, ok := c.registry.LookupByName(stripped); ok && target != fromKey {
					edgeKey := fromKey + "|" + target
					if seen[edgeKey] {
						continue
					}
					seen[edgeKey] = true
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       target,
						Type:     model.HTTPCall,
						Evidence: cu.Key + " = " + cu.URL,
					})
					continue
				}
			}

			// K8s DNS: <svc>.<ns>.svc.cluster.local
			if strings.Contains(hostname, ".svc") || strings.HasSuffix(hostname, ".local") {
				first := hostname
				if dot := strings.IndexByte(hostname, '.'); dot > 0 {
					first = hostname[:dot]
				}
				if target, ok := c.registry.LookupByName(first); ok && target != fromKey {
					edgeKey := fromKey + "|" + target
					if seen[edgeKey] {
						continue
					}
					seen[edgeKey] = true
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       target,
						Type:     model.HTTPCall,
						Evidence: cu.Key + " = " + cu.URL,
					})
					continue
				}
			}

			if sys := MatchHost(hostname); sys != nil {
				extKey := ExternalNodeName(sys)
				edgeKey := fromKey + "|" + extKey
				if !seen[edgeKey] {
					seen[edgeKey] = true
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       extKey,
						Type:     model.ExternalCall,
						Evidence: cu.Key + " = " + cu.URL,
					})
					if !externalSet[extKey] {
						externalSet[extKey] = true
						externalNodes = append(externalNodes, model.Node{
							Key:      extKey,
							Name:     sys.Label,
							Platform: "external",
						})
					}
				}
			} else if sys := MatchConfigRef(cu.Key); sys != nil {
				extKey := ExternalNodeName(sys)
				edgeKey := fromKey + "|" + extKey
				if !seen[edgeKey] {
					seen[edgeKey] = true
					edges = append(edges, model.Edge{
						From:     fromKey,
						To:       extKey,
						Type:     model.ExternalCall,
						Evidence: cu.Key + " = " + cu.URL,
					})
					if !externalSet[extKey] {
						externalSet[extKey] = true
						externalNodes = append(externalNodes, model.Node{
							Key:      extKey,
							Name:     sys.Label,
							Platform: "external",
						})
					}
				}
			} else {
				apiName := configExtractAPIName(hostname)
				if apiName != "" {
					extKey := "external:" + apiName
					edgeKey := fromKey + "|" + extKey
					if !seen[edgeKey] {
						seen[edgeKey] = true
						edges = append(edges, model.Edge{
							From:     fromKey,
							To:       extKey,
							Type:     model.HTTPCall,
							Evidence: cu.Key + " = " + cu.URL,
						})
						if !externalSet[extKey] {
							externalSet[extKey] = true
							externalNodes = append(externalNodes, model.Node{
								Key:      extKey,
								Name:     apiName,
								Platform: "external",
							})
						}
					}
				}
			}
		}
	}
	return edges, externalNodes
}

func configExtractHost(url string) string {
	s := url
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "lb://")
	if idx := strings.IndexByte(s, ':'); idx > 0 {
		s = s[:idx]
	}
	if idx := strings.IndexByte(s, '/'); idx > 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "$") {
		return ""
	}
	return strings.ToLower(s)
}

func configExtractAPIName(hostname string) string {
	parts := strings.Split(hostname, ".")
	if len(parts) == 0 {
		return ""
	}
	name := parts[0]
	if name == "" || len(name) < 3 {
		return ""
	}
	return name
}
