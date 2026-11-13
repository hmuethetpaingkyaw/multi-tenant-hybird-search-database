package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "multi-tenant-hybrid-search/internal/domain/embedding"
)

func vec(n int) []float32 { return make([]float32, n) }

func serve(t *testing.T, h http.HandlerFunc) *Ollama {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewOllama(srv.URL, "test-model", 2*time.Second)
}

func reply(code int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func TestOllamaRequestShapeAndPrefixes(t *testing.T) {
	var got struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	var path, method string
	o := serve(t, func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{vec(768)}})
	})

	if v, err := o.EmbedDocument(context.Background(), "hello"); err != nil || len(v) != 768 {
		t.Fatalf("document: len=%d err=%v", len(v), err)
	}
	if method != "POST" || path != "/api/embed" || got.Model != "test-model" || len(got.Input) != 1 || got.Input[0] != "search_document: hello" {
		t.Errorf("document request: %s %s %+v", method, path, got)
	}
	if _, err := o.EmbedQuery(context.Background(), "q"); err != nil || got.Input[0] != "search_query: q" {
		t.Errorf("query request: %+v err=%v", got, err)
	}
	if o.Model() != "test-model" {
		t.Errorf("Model() = %q", o.Model())
	}
}

func TestOllamaFailuresAreUnavailable(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"http 500":        reply(500, map[string]string{"error": "boom"}),
		"model not found": reply(404, map[string]string{"error": `model "test-model" not found, try pulling it first`}),
		"malformed json":  func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{not json")) },
		"wrong dimension": reply(200, map[string]any{"embeddings": [][]float32{vec(1024)}}),
		"no embeddings":   reply(200, map[string]any{"embeddings": [][]float32{}}),
		"two embeddings":  reply(200, map[string]any{"embeddings": [][]float32{vec(768), vec(768)}}),
		"timeout": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		},
	}
	for name, h := range cases {
		o := serve(t, h)
		if name == "timeout" {
			o.client.Timeout = 100 * time.Millisecond
		}
		_, err := o.EmbedDocument(context.Background(), "x")
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("%s: want ErrUnavailable, got %v", name, err)
		}
	}
}

func TestOllamaCauseIsKeptForLogs(t *testing.T) {
	o := serve(t, reply(200, map[string]any{"embeddings": [][]float32{vec(1024)}}))
	_, err := o.EmbedQuery(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "unexpected embedding dimension 1024") {
		t.Errorf("cause missing from error: %v", err)
	}
	if o2 := NewOllama("http://127.0.0.1:1", "m", time.Second); true {
		if _, err := o2.EmbedQuery(context.Background(), "x"); !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("connection refused: %v", err)
		}
	}
}

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
	if len(a1) != domain.Dimensions || cosine(a1, a2) < 0.9999 {
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
