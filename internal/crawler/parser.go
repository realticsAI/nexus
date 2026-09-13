package crawler

import "github.com/anurag/nexus/model"

// Parser is the interface every language extractor must implement.
// Each parser knows how to turn source files into model.Unit entries.
type Parser interface {
	Platform() model.Platform
	ParseFile(path string, content []byte) ([]model.Unit, error)
	Extensions() []string
}
