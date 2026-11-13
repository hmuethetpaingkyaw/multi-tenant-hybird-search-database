// Package embedding contains embedder adapters.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domain "multi-tenant-hybrid-search/internal/domain/embedding"
)

// nomic-embed-text expects a task prefix on every input.
const (
	documentPrefix = "search_document: "
	queryPrefix    = "search_query: "
)

// Ollama calls a local Ollama server's /api/embed endpoint.
type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllama(baseURL, model string, timeout time.Duration) *Ollama {
	return &Ollama{baseURL: strings.TrimRight(baseURL, "/"), model: model, client: &http.Client{Timeout: timeout}}
}

func (o *Ollama) Model() string { return o.model }

func (o *Ollama) EmbedDocument(ctx context.Context, text string) ([]float32, error) {
	return o.embed(ctx, documentPrefix+text)
}

func (o *Ollama) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return o.embed(ctx, queryPrefix+text)
}

func (o *Ollama) embed(ctx context.Context, input string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{"model": o.model, "input": []string{input}})
	if err != nil {
		return nil, domain.Unavailable(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, domain.Unavailable(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, domain.Unavailable(fmt.Errorf("ollama request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, domain.Unavailable(fmt.Errorf("ollama returned HTTP %d (model %q; is it pulled?): %s",
			resp.StatusCode, o.model, strings.TrimSpace(string(snippet))))
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, domain.Unavailable(fmt.Errorf("ollama response: %w", err))
	}
	if len(out.Embeddings) != 1 {
		return nil, domain.Unavailable(fmt.Errorf("ollama returned %d embeddings, want 1", len(out.Embeddings)))
	}
	if n := len(out.Embeddings[0]); n != domain.Dimensions {
		return nil, domain.Unavailable(fmt.Errorf("unexpected embedding dimension %d, want %d (model %q)", n, domain.Dimensions, o.model))
	}
	return out.Embeddings[0], nil
}
