package response

import (
	"encoding/json"
	"testing"
	"time"

	"multi-tenant-hybrid-search/domain"
)

func js(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var ts = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestDocumentJSONShapeAndKeyOrder(t *testing.T) {
	got := js(t, ToDocumentDTO(domain.Document{ID: "1", Title: "t", Content: "c", CreatedAt: ts, UpdatedAt: ts}))
	want := `{"id":"1","title":"t","content":"c","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}`
	if got != want {
		t.Errorf("got %s", got)
	}
}

func TestDocumentListNeverNullAndOmitsContent(t *testing.T) {
	if got := js(t, ToDocumentListResponse(nil, 20, 0)); got != `{"documents":[],"limit":20,"offset":0}` {
		t.Errorf("empty list: %s", got)
	}
	got := js(t, ToDocumentListResponse([]domain.DocumentSummary{{ID: "1", Title: "t", CreatedAt: ts, UpdatedAt: ts}}, 100, 5))
	want := `{"documents":[{"id":"1","title":"t","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T03:04:05Z"}],"limit":100,"offset":5}`
	if got != want {
		t.Errorf("list: %s", got)
	}
}

func TestSearchResponse(t *testing.T) {
	if got := js(t, ToSearchResponse(nil)); got != `{"results":[],"total":0}` {
		t.Errorf("empty: %s", got)
	}
	got := js(t, ToSearchResponse([]domain.SearchResult{{DocumentID: "d", Title: "t", Score: 0.5}}))
	if got != `{"results":[{"document_id":"d","title":"t","score":0.5}],"total":1}` {
		t.Errorf("one: %s", got)
	}
}

func TestMeResponseKeyOrder(t *testing.T) {
	got := js(t, ToMeResponse(domain.Tenant{ID: "t1", Name: "n"}, domain.User{ID: "u1", Email: "e@x"}))
	if got != `{"tenant":{"id":"t1","name":"n"},"user":{"email":"e@x","id":"u1"}}` {
		t.Errorf("got %s", got)
	}
}

func TestErrorAndStatusBodies(t *testing.T) {
	if got := js(t, ErrorResponse{Error: "not found"}); got != `{"error":"not found"}` {
		t.Errorf("error: %s", got)
	}
	if got := js(t, StatusResponse{Status: "ok"}); got != `{"status":"ok"}` {
		t.Errorf("status: %s", got)
	}
}
