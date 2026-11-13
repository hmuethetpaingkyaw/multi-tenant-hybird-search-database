// Package document contains the document use cases.
package document

import (
	"context"
	"errors"

	domain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/embedding"
	"multi-tenant-hybrid-search/internal/domain/tenant"
)

// MaxEmbedChars bounds the text that is embedded: one chunk per document, so only the first
// MaxEmbedChars characters of title + content influence semantic search. Real chunking comes later.
const MaxEmbedChars = 8000

type Service struct {
	repo domain.Repository
	emb  embedding.Embedder
}

func NewService(repo domain.Repository, emb embedding.Embedder) *Service {
	return &Service{repo: repo, emb: emb}
}

// embed builds the chunk and embeds it. It runs before any database transaction, so a slow embedding
// service never holds a pooled connection. Any failure is reported as embedding.ErrUnavailable.
func (s *Service) embed(ctx context.Context, in domain.Input) (embedding.Embedding, error) {
	chunk := buildChunk(in)
	vec, err := s.emb.EmbedDocument(ctx, chunk)
	if err != nil {
		if !errors.Is(err, embedding.ErrUnavailable) {
			err = embedding.Unavailable(err)
		}
		return embedding.Embedding{}, err
	}
	return embedding.Embedding{Model: s.emb.Model(), Content: chunk, Vector: vec}, nil
}

// buildChunk is title + blank line + content, cut to MaxEmbedChars characters.
func buildChunk(in domain.Input) string {
	r := []rune(in.Title + "\n\n" + in.Content)
	if len(r) > MaxEmbedChars {
		r = r[:MaxEmbedChars]
	}
	return string(r)
}

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
	emb, err := s.embed(ctx, v)
	if err != nil {
		return domain.Document{}, err
	}
	return s.repo.Create(ctx, id.TenantID, v, emb)
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

// Update replaces title and content and re-embeds. Embedding happens first, so if the embedding
// service is down the update fails and the document and its embedding stay unchanged.
func (s *Service) Update(ctx context.Context, id tenant.Identity, docID string, in domain.Input) (domain.Document, error) {
	v, err := in.Validate()
	if err != nil {
		return domain.Document{}, err
	}
	emb, err := s.embed(ctx, v)
	if err != nil {
		return domain.Document{}, err
	}
	return s.repo.Update(ctx, id.TenantID, docID, v, emb)
}

func (s *Service) Delete(ctx context.Context, id tenant.Identity, docID string) error {
	return s.repo.Delete(ctx, id.TenantID, docID)
}
