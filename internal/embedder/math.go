package embedder

import "math"

func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func TopK(query []float32, vectors [][]float32, labels []string, k int) []ScoredMatch {
	var results []ScoredMatch
	for i, vec := range vectors {
		score := CosineSimilarity(query, vec)
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		results = append(results, ScoredMatch{Label: label, Score: score, Index: i})
	}
	sortScored(results)
	if k > 0 && len(results) > k {
		results = results[:k]
	}
	return results
}

type ScoredMatch struct {
	Label string  `json:"label"`
	Score float32 `json:"score"`
	Index int     `json:"index"`
}

func sortScored(s []ScoredMatch) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Score > s[j-1].Score; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
