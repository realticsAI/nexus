package analyzer

import "github.com/anurag/nexus/model"

type KafkaEdgeResolver struct{}

func NewKafkaEdgeResolver() *KafkaEdgeResolver {
	return &KafkaEdgeResolver{}
}

func (k *KafkaEdgeResolver) Resolve(services map[string]*model.ServiceIndex) []model.Edge {
	producers := make(map[string][]string)
	consumers := make(map[string][]string)

	for key, svc := range services {
		for _, unit := range svc.Units {
			for _, topic := range unit.KafkaProduces {
				producers[topic] = append(producers[topic], key)
			}
			for _, topic := range unit.KafkaConsumes {
				consumers[topic] = append(consumers[topic], key)
			}
		}
	}

	var edges []model.Edge
	for topic, prods := range producers {
		cons, ok := consumers[topic]
		if !ok {
			continue
		}
		for _, p := range prods {
			for _, c := range cons {
				if p == c {
					continue
				}
				edges = append(edges, model.Edge{
					From:     p,
					To:       c,
					Type:     model.KafkaProduce,
					Evidence: "topic: " + topic,
				})
			}
		}
	}
	return edges
}
