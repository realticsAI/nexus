package embedder

import (
	"fmt"
	"strings"
	"sync"

	"github.com/anurag/nexus/model"
)

type Embedder struct {
	provider    Provider
	store       *Store
	concurrency int
}

func NewEmbedder(provider Provider, store *Store, concurrency int) *Embedder {
	if concurrency <= 0 {
		concurrency = 2
	}
	return &Embedder{provider: provider, store: store, concurrency: concurrency}
}

func (e *Embedder) EmbedServices(services map[string]*model.ServiceIndex) error {
	if e.provider == nil || !e.provider.Available() {
		return fmt.Errorf("embedding provider not available")
	}

	sem := make(chan struct{}, e.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []string

	for key, svc := range services {
		wg.Add(1)
		sem <- struct{}{}
		go func(k string, s *model.ServiceIndex) {
			defer wg.Done()
			defer func() { <-sem }()

			texts, labels := serviceToTexts(s)
			if len(texts) == 0 {
				return
			}

			vectors, err := e.provider.Embed(texts)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Sprintf("%s: %v", k, err))
				mu.Unlock()
				return
			}

			e.store.Save(k, vectors, labels)
		}(key, svc)
	}
	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("embedding errors: %s", strings.Join(errors, "; "))
	}
	return nil
}

func serviceToTexts(svc *model.ServiceIndex) ([]string, []string) {
	var texts, labels []string
	for _, unit := range svc.Units {
		text := unit.Type + " " + unit.Name
		if unit.Stereotype != "" {
			text += " (" + unit.Stereotype + ")"
		}
		for _, m := range unit.Methods {
			text += " " + m.Name
		}
		if len(unit.Endpoints) > 0 {
			text += " endpoints: " + strings.Join(unit.Endpoints, ", ")
		}
		texts = append(texts, text)
		labels = append(labels, unit.Name)
	}
	return texts, labels
}

func (e *Embedder) Provider() Provider { return e.provider }
func (e *Embedder) Store() *Store      { return e.store }
