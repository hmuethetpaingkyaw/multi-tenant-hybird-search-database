// Package search contains the search use cases.
package search

import (
	"context"
	"errors"

	"multi-tenant-hybrid-search/internal/domain/embedding"
	domain "multi-tenant-hybrid-search/internal/domain/search"
	"multi-tenant-hybrid-search/internal/domain/tenant"
)

type Service struct {
	repo domain.Repository
	emb  embedding.Embedder
}

func NewService(repo domain.Repository, emb embedding.Embedder) *Service {
	return &Service{repo: repo, emb: emb}
}

// Semantic finds the caller's documents closest in meaning to query. The query is embedded before
// any database transaction opens; embedding failures are reported as embedding.ErrUnavailable.
func (s *Service) Semantic(ctx context.Context, id tenant.Identity, query string, limit *int) ([]domain.Result, error) {
	q, err := domain.ValidateQuery(query)
	if err != nil {
		return nil, err
	}
	l, err := domain.NormalizeLimit(limit)
	if err != nil {
		return nil, err
	}
	vec, err := s.emb.EmbedQuery(ctx, q)
	if err != nil {
		if !errors.Is(err, embedding.ErrUnavailable) {
			err = embedding.Unavailable(err)
		}
		return nil, err
	}
	res, err := s.repo.Semantic(ctx, id.TenantID, vec, l)
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = []domain.Result{}
	}
	return res, nil
}
