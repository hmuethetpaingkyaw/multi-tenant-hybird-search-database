package response

import "multi-tenant-hybrid-search/domain"

type SearchResultDTO struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	Score      float64 `json:"score"`
}

type SearchResponse struct {
	Results []SearchResultDTO `json:"results"`
	Total   int               `json:"total"`
}

// ToSearchResponse never returns a nil Results slice, so no matches is [] and not null.
func ToSearchResponse(results []domain.SearchResult) SearchResponse {
	items := make([]SearchResultDTO, 0, len(results))
	for _, r := range results {
		items = append(items, SearchResultDTO{DocumentID: r.DocumentID, Title: r.Title, Score: r.Score})
	}
	return SearchResponse{Results: items, Total: len(items)}
}
