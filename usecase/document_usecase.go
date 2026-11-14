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

type DocumentUsecase struct {
	documentPort repository.DocumentPort
	embedder     ollama.EmbedderPort
}

func NewDocumentUsecase(documentPort repository.DocumentPort, embedder ollama.EmbedderPort) *DocumentUsecase {
	return &DocumentUsecase{documentPort: documentPort, embedder: embedder}
}

// DocumentListResult is one page of document summaries with the effective paging values.
type DocumentListResult struct {
	Documents []domain.DocumentSummary
	Limit     int
	Offset    int
}

// Every use case runs in the identity's tenant: the tenant comes from the authenticated identity only,
// never from the input. Input is validated by the controller (http/request) before it gets here.

// Create embeds first, then stores the document and its embedding in one transaction.
func (u *DocumentUsecase) Create(ctx context.Context, id domain.Identity, in domain.DocumentInput) (domain.Document, error) {
	emb, err := u.embed(ctx, in)
	if err != nil {
		return domain.Document{}, err
	}
	return u.documentPort.Create(ctx, id.TenantID, in, emb)
}

func (u *DocumentUsecase) Get(ctx context.Context, id domain.Identity, docID string) (domain.Document, error) {
	return u.documentPort.Get(ctx, id.TenantID, docID)
}

// List returns the caller's documents, newest first. nil limit/offset mean "use defaults";
// a limit above the maximum is clamped.
func (u *DocumentUsecase) List(ctx context.Context, id domain.Identity, limit, offset *int) (DocumentListResult, error) {
	l, o := constants.DefaultListLimit, 0
	if limit != nil {
		if *limit < 1 {
			return DocumentListResult{}, apperror.NewValidationError("%s", constants.MsgLimitInvalid)
		}
		l = min(*limit, constants.MaxListLimit)
	}
	if offset != nil {
		if *offset < 0 {
			return DocumentListResult{}, apperror.NewValidationError("%s", constants.MsgOffsetInvalid)
		}
		o = *offset
	}
	docs, err := u.documentPort.List(ctx, id.TenantID, l, o)
	if err != nil {
		return DocumentListResult{}, err
	}
	return DocumentListResult{Documents: docs, Limit: l, Offset: o}, nil
}

// Update replaces title and content and re-embeds. Embedding happens first, so if the embedding
// service is down the update fails and the document and its embedding stay unchanged.
func (u *DocumentUsecase) Update(ctx context.Context, id domain.Identity, docID string, in domain.DocumentInput) (domain.Document, error) {
	emb, err := u.embed(ctx, in)
	if err != nil {
		return domain.Document{}, err
	}
	return u.documentPort.Update(ctx, id.TenantID, docID, in, emb)
}

func (u *DocumentUsecase) Delete(ctx context.Context, id domain.Identity, docID string) error {
	return u.documentPort.Delete(ctx, id.TenantID, docID)
}

// embed builds the chunk and embeds it. It runs before any database transaction, so a slow embedding
// service never holds a pooled connection. Any failure is reported as apperror.ErrEmbeddingUnavailable.
func (u *DocumentUsecase) embed(ctx context.Context, in domain.DocumentInput) (domain.DocumentEmbedding, error) {
	chunk := buildChunk(in)
	vec, err := u.embedder.EmbedDocument(ctx, chunk)
	if err != nil {
		if !errors.Is(err, apperror.ErrEmbeddingUnavailable) {
			err = apperror.Unavailable(err)
		}
		return domain.DocumentEmbedding{}, err
	}
	return domain.DocumentEmbedding{Model: u.embedder.Model(), Content: chunk, Vector: vec}, nil
}

// buildChunk is title + blank line + content, cut to constants.MaxEmbedChars characters.
func buildChunk(in domain.DocumentInput) string {
	r := []rune(in.Title + "\n\n" + in.Content)
	if len(r) > constants.MaxEmbedChars {
		r = r[:constants.MaxEmbedChars]
	}
	return string(r)
}
