// Package mock_repository holds hand-written fakes of the repository Ports for usecase tests.
package mock_repository

import (
	"context"

	"multi-tenant-hybrid-search/domain"
)

// MockDocumentPort records calls and returns Err (and canned values) from every method.
type MockDocumentPort struct {
	Calls  int    // number of repository calls
	Scope  string // tenant id of the last call
	Limit  int    // last List limit
	Offset int    // last List offset
	Err    error  // returned by every method
}

func (m *MockDocumentPort) rec(scope string) { m.Calls++; m.Scope = scope }

func (m *MockDocumentPort) Create(_ context.Context, scope string, in domain.DocumentInput, _ domain.DocumentEmbedding) (domain.Document, error) {
	m.rec(scope)
	return domain.Document{Title: in.Title}, m.Err
}

func (m *MockDocumentPort) Get(_ context.Context, scope, _ string) (domain.Document, error) {
	m.rec(scope)
	return domain.Document{}, m.Err
}

func (m *MockDocumentPort) List(_ context.Context, scope string, limit, offset int) ([]domain.DocumentSummary, error) {
	m.rec(scope)
	m.Limit, m.Offset = limit, offset
	return []domain.DocumentSummary{}, m.Err
}

func (m *MockDocumentPort) Update(_ context.Context, scope, _ string, in domain.DocumentInput, _ domain.DocumentEmbedding) (domain.Document, error) {
	m.rec(scope)
	return domain.Document{Title: in.Title}, m.Err
}

func (m *MockDocumentPort) Delete(_ context.Context, scope, _ string) error {
	m.rec(scope)
	return m.Err
}
