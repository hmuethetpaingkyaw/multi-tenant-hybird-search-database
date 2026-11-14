// Package mock_ollama holds the test-only fake embedder.
package mock_ollama

import (
	"context"
	"hash/fnv"
	"math"
	"multi-tenant-hybrid-search/constants"
	"strings"
	"unicode"
)

// Fake is a deterministic, test-only embedder: a hashed bag of words, L2-normalized, so texts that
// share words get similar vectors. Set Err to simulate an embedding outage.
type Fake struct {
	Err   error
	Calls int
}

func (f *Fake) Model() string { return "fake-bow-768" }

func (f *Fake) EmbedDocument(_ context.Context, text string) ([]float32, error) { return f.embed(text) }
func (f *Fake) EmbedQuery(_ context.Context, text string) ([]float32, error)    { return f.embed(text) }

func (f *Fake) embed(text string) ([]float32, error) {
	f.Calls++
	if f.Err != nil {
		return nil, f.Err
	}
	return Vectorize(text), nil
}

// Vectorize is the fake embedding function, exported so tests can build vectors directly.
func Vectorize(text string) []float32 {
	v := make([]float32, constants.EmbeddingDimensions)
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[h.Sum32()%constants.EmbeddingDimensions]++
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm == 0 {
		v[0] = 1
		return v
	}
	n := float32(math.Sqrt(norm))
	for i := range v {
		v[i] /= n
	}
	return v
}
