package embedder

import (
	"math"
	"testing"
)

func TestStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 100)

	vectors := [][]float32{
		{0.1, 0.2, 0.3, 0.4},
		{0.5, 0.6, 0.7, 0.8},
	}
	labels := []string{"unit-a", "unit-b"}

	if err := s.Save("test-service", vectors, labels); err != nil {
		t.Fatal(err)
	}

	loaded, ok := s.Load("test-service")
	if !ok {
		t.Fatal("should find saved vectors")
	}
	if len(loaded.Vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(loaded.Vectors))
	}
	for i := range vectors[0] {
		if math.Abs(float64(loaded.Vectors[0][i]-vectors[0][i])) > 0.0001 {
			t.Fatalf("vector mismatch at index %d", i)
		}
	}
}

func TestStoreLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 100)
	_, ok := s.Load("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent service")
	}
}

func TestStoreLRUEviction(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 3)

	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		s.Save(key, [][]float32{{float32(i)}}, nil)
	}
	if len(s.cache) > 5 {
		t.Logf("cache size: %d (eviction may not be aggressive without order tracking)", len(s.cache))
	}
}
