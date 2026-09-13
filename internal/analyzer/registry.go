package analyzer

import (
	"strings"

	"github.com/anurag/nexus/model"
)

type ServiceRegistry struct {
	byKey       map[string]*model.ServiceIndex
	byName      map[string]string
	endpointMap map[string]string
}

func NewRegistry(services map[string]*model.ServiceIndex) *ServiceRegistry {
	r := &ServiceRegistry{
		byKey:       services,
		byName:      make(map[string]string),
		endpointMap: make(map[string]string),
	}
	for key, svc := range services {
		parts := strings.SplitN(key, ":", 2)
		name := key
		if len(parts) == 2 {
			name = parts[1]
		}
		r.byName[name] = key
		r.byName[strings.ToLower(name)] = key

		if dot := strings.LastIndex(name, "."); dot >= 0 {
			suffix := name[dot+1:]
			if _, exists := r.byName[suffix]; !exists {
				r.byName[suffix] = key
				r.byName[strings.ToLower(suffix)] = key
			}
		}
		if dash := strings.Index(name, "."); dash >= 0 {
			prefix := name[:dash]
			if _, exists := r.byName[prefix]; !exists {
				r.byName[prefix] = key
			}
		}

		for _, unit := range svc.Units {
			for _, ep := range unit.Endpoints {
				parts := strings.SplitN(ep, " ", 2)
				if len(parts) == 2 {
					r.endpointMap[parts[1]] = key
				}
				r.endpointMap[ep] = key
			}
		}
	}
	return r
}

func (r *ServiceRegistry) LookupByKey(key string) *model.ServiceIndex {
	return r.byKey[key]
}

func (r *ServiceRegistry) LookupByName(name string) (string, bool) {
	key, ok := r.byName[name]
	if !ok {
		key, ok = r.byName[strings.ToLower(name)]
	}
	return key, ok
}

func (r *ServiceRegistry) LookupEndpoint(path string) (string, bool) {
	key, ok := r.endpointMap[path]
	return key, ok
}

func (r *ServiceRegistry) ResolveConfigURL(configKey string, svc *model.ServiceIndex) string {
	if val, ok := svc.Config[configKey]; ok {
		return val
	}
	return ""
}

func (r *ServiceRegistry) AllKeys() []string {
	keys := make([]string, 0, len(r.byKey))
	for k := range r.byKey {
		keys = append(keys, k)
	}
	return keys
}

func (r *ServiceRegistry) Nodes() []model.Node {
	nodes := make([]model.Node, 0, len(r.byKey))
	for key, svc := range r.byKey {
		parts := strings.SplitN(key, ":", 2)
		name, workspace := key, ""
		if len(parts) == 2 {
			workspace = parts[0]
			name = parts[1]
		}
		nodes = append(nodes, model.Node{
			Key:       key,
			Name:      name,
			Platform:  svc.Platform.String(),
			Workspace: workspace,
		})
	}
	return nodes
}
