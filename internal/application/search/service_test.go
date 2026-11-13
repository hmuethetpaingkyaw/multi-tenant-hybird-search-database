package search

import (
	"context"
	"errors"
	"testing"

	"multi-tenant-hybrid-search/internal/domain/embedding"
	domain "multi-tenant-hybrid-search/internal/domain/search"
	"multi-tenant-hybrid-search/internal/domain/tenant"
	emb "multi-tenant-hybrid-search/internal/infrastructure/embedding"
)

type fakeRepo struct {
	calls int
	scope string
	limit int
	vec   []float32
	res   []domain.Result
	err   error
}

func (f *fakeRepo) Semantic(_ context.Context, scope string, v []float32, limit int) ([]domain.Result, error) {
	f.calls++
	f.scope, f.vec, f.limit = scope, v, limit
	return f.res, f.err
}

var ident = tenant.Identity{UserID: "u", TenantID: "11111111-1111-1111-1111-111111111111"}

func TestValidationAndPaginationNeverReachDependencies(t *testing.T) {
	zero, neg := 0, -3
	cases := []struct {
		name  string
		query string
		limit *int
		msg   string
	}{
		{"empty query", "", nil, "query is required"},
		{"blank query", "   ", nil, "query is required"},
		{"too long", string(make([]rune, 2001)), nil, "query must be at most 2000 characters"},
		{"limit 0", "ok", &zero, domain.MsgLimit},
		{"limit negative", "ok", &neg, domain.MsgLimit},
	}
	for _, c := range cases {
		if c.name == "too long" {
			c.query = ""
			for i := 0; i < 2001; i++ {
				c.query += "a"
			}
		}
		r, f := &fakeRepo{}, &emb.Fake{}
		_, err := NewService(r, f).Semantic(context.Background(), ident, c.query, c.limit)
		if !errors.Is(err, domain.ErrInvalid) || err.Error() != c.msg {
			t.Errorf("%s: err=%v, want ErrInvalid %q", c.name, err, c.msg)
		}
		if r.calls != 0 || f.Calls != 0 {
			t.Errorf("%s: dependencies were called (repo %d, embedder %d)", c.name, r.calls, f.Calls)
		}
	}
}

func TestLimitDefaultsClampAndScope(t *testing.T) {
	big := 1000
	for _, c := range []struct {
		limit *int
		want  int
	}{{nil, 10}, {&big, 100}} {
		r := &fakeRepo{}
		res, err := NewService(r, &emb.Fake{}).Semantic(context.Background(), ident, "  recover account ", c.limit)
		if err != nil || r.limit != c.want || r.scope != ident.TenantID || len(r.vec) != 768 {
			t.Errorf("limit %v: repo limit=%d scope=%q err=%v", c.limit, r.limit, r.scope, err)
		}
		if res == nil {
			t.Error("result must be a non-nil empty slice")
		}
	}
}

func TestEmbeddingFailureIsUnavailableAndSkipsDatabase(t *testing.T) {
	r := &fakeRepo{}
	f := &emb.Fake{Err: errors.New("down")}
	_, err := NewService(r, f).Semantic(context.Background(), ident, "q", nil)
	if !errors.Is(err, embedding.ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
	if r.calls != 0 {
		t.Error("repository must not be called when embedding fails")
	}
}
