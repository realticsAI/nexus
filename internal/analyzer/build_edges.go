package analyzer

import "github.com/anurag/nexus/model"

type BuildDepResolver struct {
	registry *ServiceRegistry
}

func NewBuildDepResolver(r *ServiceRegistry) *BuildDepResolver {
	return &BuildDepResolver{registry: r}
}

func (b *BuildDepResolver) Resolve(services map[string]*model.ServiceIndex) []model.Edge {
	var edges []model.Edge

	for fromKey, svc := range services {
		for _, dep := range svc.BuildDeps {
			if target, ok := b.registry.LookupByName(dep); ok && target != fromKey {
				edges = append(edges, model.Edge{
					From:     fromKey,
					To:       target,
					Type:     model.BuildDependency,
					Evidence: "build dep: " + dep,
				})
			}
		}
	}
	return edges
}
