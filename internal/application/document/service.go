// Package document contains the document use cases.
package document

import (
	"context"

	domain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/tenant"
)

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

// ListResult is one page of document summaries with the effective paging values.
type ListResult struct {
	Documents []domain.Summary
	Limit     int
	Offset    int
}

// Every use case runs in the identity's tenant: the tenant comes from the authenticated
// identity only, never from the input.

func (s *Service) Create(ctx context.Context, id tenant.Identity, in domain.Input) (domain.Document, error) {
	v, err := in.Validate()
	if err != nil {
		return domain.Document{}, err
	}
	return s.repo.Create(ctx, id.TenantID, v)
}

func (s *Service) Get(ctx context.Context, id tenant.Identity, docID string) (domain.Document, error) {
	return s.repo.Get(ctx, id.TenantID, docID)
}

// List returns the caller's documents, newest first. nil limit/offset mean "use defaults".
func (s *Service) List(ctx context.Context, id tenant.Identity, limit, offset *int) (ListResult, error) {
	l, o, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	docs, err := s.repo.List(ctx, id.TenantID, l, o)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Documents: docs, Limit: l, Offset: o}, nil
}

// Update replaces title and content.
// Phase 4: when embeddings exist, a content change must re-embed; nothing to do yet.
func (s *Service) Update(ctx context.Context, id tenant.Identity, docID string, in domain.Input) (domain.Document, error) {
	v, err := in.Validate()
	if err != nil {
		return domain.Document{}, err
	}
	return s.repo.Update(ctx, id.TenantID, docID, v)
}

func (s *Service) Delete(ctx context.Context, id tenant.Identity, docID string) error {
	return s.repo.Delete(ctx, id.TenantID, docID)
}
