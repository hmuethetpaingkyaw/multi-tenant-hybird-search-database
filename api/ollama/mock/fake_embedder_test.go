package mock_ollama

import (
	"context"
	"errors"
	"testing"

	"multi-tenant-hybrid-search/constants"
)

func cosine(a, b []float32) float64 {
	var d float64
	for i := range a {
		d += float64(a[i]) * float64(b[i])
	}
	return d
}

func TestFakeIsDeterministicAndRanksOverlapCloser(t *testing.T) {
	f := &Fake{}
	a1, _ := f.EmbedDocument(context.Background(), "how to recover your account password")
	a2, _ := f.EmbedDocument(context.Background(), "how to recover your account password")
	if len(a1) != constants.EmbeddingDimensions || cosine(a1, a2) < 0.9999 {
		t.Fatal("fake must be deterministic with 768 dims")
	}
	q, _ := f.EmbedQuery(context.Background(), "recover my account")
	related := cosine(q, a1)
	unrelated, _ := f.EmbedDocument(context.Background(), "quarterly sales figures for europe")
	if related <= cosine(q, unrelated) {
		t.Errorf("related %.3f should beat unrelated %.3f", related, cosine(q, unrelated))
	}
	f.Err = errors.New("down")
	if _, err := f.EmbedDocument(context.Background(), "x"); err == nil {
		t.Error("Err must be returned")
	}
}
