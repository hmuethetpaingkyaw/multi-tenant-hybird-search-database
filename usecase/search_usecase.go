package usecase

import (
	"context"
	"errors"

	"multi-tenant-hybrid-search/api/ollama"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/repository"
)

type SearchUsecase struct {
	searchPort repository.SearchPort
	embedder   ollama.EmbedderPort
}

func NewSearchUsecase(searchPort repository.SearchPort, embedder ollama.EmbedderPort) *SearchUsecase {
	return &SearchUsecase{searchPort: searchPort, embedder: embedder}
}

// Semantic finds the caller's documents closest in meaning to query. The query is embedded before any
// database transaction opens; embedding failures are reported as apperror.ErrEmbeddingUnavailable.
// A nil limit means the default; a limit above the maximum is clamped.
func (u *SearchUsecase) Semantic(ctx context.Context, id domain.Identity, query string, limit *int) ([]domain.SearchResult, error) {
	l := constants.DefaultSearchLimit
	if limit != nil {
		if *limit < 1 {
			return nil, apperror.NewValidationError("%s", constants.MsgLimitInvalid)
		}
		l = min(*limit, constants.MaxSearchLimit)
	}
	vec, err := u.embedder.EmbedQuery(ctx, query)
	if err != nil {
		if !errors.Is(err, apperror.ErrEmbeddingUnavailable) {
			err = apperror.Unavailable(err)
		}
		return nil, err
	}
	res, err := u.searchPort.Semantic(ctx, id.TenantID, vec, l)
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = []domain.SearchResult{}
	}
	return res, nil
}
