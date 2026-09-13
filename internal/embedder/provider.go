package embedder

type Provider interface {
	Embed(texts []string) ([][]float32, error)
	Dimensions() int
	Available() bool
}
