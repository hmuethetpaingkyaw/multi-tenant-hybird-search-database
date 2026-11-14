package domain

// SearchResult is one matching document. Score is cosine similarity (higher is better).
type SearchResult struct {
	DocumentID string
	Title      string
	Score      float64
}
