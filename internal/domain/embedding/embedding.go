// Package embedding defines the contract for turning text into vectors.
package embedding

import (
	"context"
	"errors"
	"fmt"
)

// Dimensions is fixed by the schema (document_embeddings.embedding is vector(768)).
const Dimensions = 768

// ErrUnavailable means the embedding service failed (down, timeout, bad response, wrong model/dimension).
// Callers show a generic message; the wrapped cause is for logs only.
var ErrUnavailable = errors.New("embedding service unavailable")

// Unavailable wraps cause so that errors.Is(err, ErrUnavailable) holds and the cause stays readable in logs.
func Unavailable(cause error) error { return fmt.Errorf("%w: %w", ErrUnavailable, cause) }

// Embedder turns text into vectors. Document and query texts are embedded differently
// by some models, so the two roles are separate; model-specific prefixes live in the adapter.
type Embedder interface {
	Model() string
	EmbedDocument(ctx context.Context, text string) ([]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// Embedding is what is stored for one chunk of a document.
type Embedding struct {
	Model   string
	Content string // the exact chunk text that was embedded (without model prefixes)
	Vector  []float32
}
