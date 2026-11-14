package mock_repository

import (
	"context"

	"multi-tenant-hybrid-search/domain"
)

// MockSearchPort records the last call and returns Results/Err.
type MockSearchPort struct {
	Calls   int
	Scope   string
	Limit   int
	Vector  []float32
	Results []domain.SearchResult
	Err     error
}

func (m *MockSearchPort) Semantic(_ context.Context, scope string, v []float32, limit int) ([]domain.SearchResult, error) {
	m.Calls++
	m.Scope, m.Vector, m.Limit = scope, v, limit
	return m.Results, m.Err
}
