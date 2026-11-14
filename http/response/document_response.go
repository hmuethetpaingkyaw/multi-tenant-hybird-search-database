// Package response holds the HTTP response bodies (DTOs) and their converters.
package response

import (
	"time"

	"multi-tenant-hybrid-search/domain"
)

type DocumentDTO struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DocumentSummaryDTO struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DocumentListResponse is one page of summaries with the effective paging values.
type DocumentListResponse struct {
	Documents []DocumentSummaryDTO `json:"documents"`
	Limit     int                  `json:"limit"`
	Offset    int                  `json:"offset"`
}

func ToDocumentDTO(d domain.Document) DocumentDTO {
	return DocumentDTO{ID: d.ID, Title: d.Title, Content: d.Content, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

// ToDocumentListResponse never returns a nil Documents slice, so an empty page is [] and not null.
func ToDocumentListResponse(docs []domain.DocumentSummary, limit, offset int) DocumentListResponse {
	items := make([]DocumentSummaryDTO, 0, len(docs))
	for _, d := range docs {
		items = append(items, DocumentSummaryDTO{ID: d.ID, Title: d.Title, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt})
	}
	return DocumentListResponse{Documents: items, Limit: limit, Offset: offset}
}
