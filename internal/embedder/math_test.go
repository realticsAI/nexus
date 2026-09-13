package embedder

import (
	"math"
	"testing"
)

func TestCosineSimilaritySame(t *testing.T) {
	a := []float32{1, 2, 3}
	score := CosineSimilarity(a, a)
	if math.Abs(float64(score)-1.0) > 0.001 {
		t.Fatalf("same vector should have similarity ~1.0, got %f", score)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	score := CosineSimilarity(a, b)
	if math.Abs(float64(score)) > 0.001 {
		t.Fatalf("orthogonal vectors should have similarity ~0, got %f", score)
	}
}

func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	score := CosineSimilarity(a, b)
	if math.Abs(float64(score)+1.0) > 0.001 {
		t.Fatalf("opposite vectors should have similarity ~-1, got %f", score)
	}
}

func TestCosineSimilarityEmpty(t *testing.T) {
	score := CosineSimilarity(nil, nil)
	if score != 0 {
		t.Fatalf("empty should return 0, got %f", score)
	}
}

func TestTopK(t *testing.T) {
	query := []float32{1, 0, 0}
	vectors := [][]float32{
		{1, 0, 0},
		{0, 1, 0},
		{0.9, 0.1, 0},
	}
	labels := []string{"exact", "orthogonal", "close"}
	results := TopK(query, vectors, labels, 2)
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
	if results[0].Label != "exact" {
		t.Fatalf("expected exact first, got %s", results[0].Label)
	}
	if results[1].Label != "close" {
		t.Fatalf("expected close second, got %s", results[1].Label)
	}
}
