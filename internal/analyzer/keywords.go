package analyzer

import (
	"strings"
	"unicode"

	"github.com/anurag/nexus/model"
)

type KeywordIndex struct {
	Index map[string][]model.Hit
}

func NewKeywordIndex() *KeywordIndex {
	return &KeywordIndex{Index: make(map[string][]model.Hit)}
}

func (ki *KeywordIndex) Build(services map[string]*model.ServiceIndex) {
	for key, svc := range services {
		parts := strings.SplitN(key, ":", 2)
		serviceName := key
		if len(parts) == 2 {
			serviceName = parts[1]
		}
		ki.addToken(serviceName, model.Hit{ServiceKey: key, UnitType: "service", UnitName: serviceName, Score: 15})
		ki.addToken(strings.ToLower(serviceName), model.Hit{ServiceKey: key, UnitType: "service", UnitName: serviceName, Score: 15})

		for _, token := range splitCamelCase(serviceName) {
			ki.addToken(strings.ToLower(token), model.Hit{ServiceKey: key, UnitType: "service", UnitName: serviceName, Score: 10})
		}

		for _, unit := range svc.Units {
			ki.addToken(unit.Name, model.Hit{ServiceKey: key, UnitType: unit.Type, UnitName: unit.Name, Score: 10})
			ki.addToken(strings.ToLower(unit.Name), model.Hit{ServiceKey: key, UnitType: unit.Type, UnitName: unit.Name, Score: 10})

			for _, token := range splitCamelCase(unit.Name) {
				ki.addToken(strings.ToLower(token), model.Hit{ServiceKey: key, UnitType: unit.Type, UnitName: unit.Name, Score: 7})
			}

			if unit.Stereotype != "" {
				ki.addToken(unit.Stereotype, model.Hit{ServiceKey: key, UnitType: unit.Type, UnitName: unit.Name, Score: 5})
			}

			for _, m := range unit.Methods {
				ki.addToken(m.Name, model.Hit{ServiceKey: key, UnitType: "method", UnitName: unit.Name + "." + m.Name, Score: 8})
				ki.addToken(strings.ToLower(m.Name), model.Hit{ServiceKey: key, UnitType: "method", UnitName: unit.Name + "." + m.Name, Score: 8})
				for _, ann := range m.Annotations {
					ann = strings.TrimPrefix(ann, "@")
					ki.addToken(ann, model.Hit{ServiceKey: key, UnitType: "annotation", UnitName: ann, Score: 5})
				}
				if m.HTTPPath != "" {
					ki.addToken(m.HTTPPath, model.Hit{ServiceKey: key, UnitType: "endpoint", UnitName: m.HTTPMethod + " " + m.HTTPPath, Score: 5})
				}
			}

			for _, ep := range unit.Endpoints {
				ki.addToken(ep, model.Hit{ServiceKey: key, UnitType: "endpoint", UnitName: ep, Score: 5})
			}

			for _, topic := range unit.KafkaProduces {
				ki.addToken(topic, model.Hit{ServiceKey: key, UnitType: "kafka_topic", UnitName: "produces:" + topic, Score: 5})
			}
			for _, topic := range unit.KafkaConsumes {
				ki.addToken(topic, model.Hit{ServiceKey: key, UnitType: "kafka_topic", UnitName: "consumes:" + topic, Score: 5})
			}
		}
	}
}

func (ki *KeywordIndex) addToken(token string, hit model.Hit) {
	if token == "" {
		return
	}
	ki.Index[token] = append(ki.Index[token], hit)
}

func (ki *KeywordIndex) Search(query string, limit int) []model.Hit {
	tokens := tokenize(query)
	scoreMap := make(map[string]*model.Hit)

	for _, token := range tokens {
		if hits, ok := ki.Index[token]; ok {
			for _, h := range hits {
				key := h.ServiceKey + "|" + h.UnitName
				if existing, ok := scoreMap[key]; ok {
					existing.Score += h.Score
				} else {
					copy := h
					scoreMap[key] = &copy
				}
			}
		}
		lower := strings.ToLower(token)
		if lower != token {
			if hits, ok := ki.Index[lower]; ok {
				for _, h := range hits {
					key := h.ServiceKey + "|" + h.UnitName
					if existing, ok := scoreMap[key]; ok {
						existing.Score += h.Score
					} else {
						copy := h
						scoreMap[key] = &copy
					}
				}
			}
		}
		for indexed := range ki.Index {
			if strings.HasPrefix(indexed, token) && indexed != token {
				for _, h := range ki.Index[indexed] {
					key := h.ServiceKey + "|" + h.UnitName
					if _, ok := scoreMap[key]; !ok {
						copy := h
						copy.Score = h.Score / 2
						scoreMap[key] = &copy
					}
				}
			}
		}
	}

	results := make([]model.Hit, 0, len(scoreMap))
	for _, h := range scoreMap {
		results = append(results, *h)
	}
	sortHits(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func tokenize(query string) []string {
	words := strings.Fields(query)
	var tokens []string
	for _, w := range words {
		tokens = append(tokens, w)
		tokens = append(tokens, splitCamelCase(w)...)
	}
	return tokens
}

func splitCamelCase(s string) []string {
	var parts []string
	var current strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
		if r == '-' || r == '_' || r == '.' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func sortHits(hits []model.Hit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}
