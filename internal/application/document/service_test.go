package document

import (
	"context"
	"errors"
	"testing"

	domain "multi-tenant-hybrid-search/internal/domain/document"
	"multi-tenant-hybrid-search/internal/domain/embedding"
	"multi-tenant-hybrid-search/internal/domain/tenant"
	emb "multi-tenant-hybrid-search/internal/infrastructure/embedding"
)

type fakeRepo struct {
	calls      int
	scope      string
	limit, off int
	err        error
}

func (f *fakeRepo) rec(scope string) { f.calls++; f.scope = scope }
func (f *fakeRepo) Create(_ context.Context, s string, in domain.Input, _ embedding.Embedding) (domain.Document, error) {
	f.rec(s)
	return domain.Document{Title: in.Title}, f.err
}
func (f *fakeRepo) Get(_ context.Context, s, _ string) (domain.Document, error) {
	f.rec(s)
	return domain.Document{}, f.err
}
func (f *fakeRepo) List(_ context.Context, s string, l, o int) ([]domain.Summary, error) {
	f.rec(s)
	f.limit, f.off = l, o
	return []domain.Summary{}, f.err
}
func (f *fakeRepo) Update(_ context.Context, s, _ string, in domain.Input, _ embedding.Embedding) (domain.Document, error) {
	f.rec(s)
	return domain.Document{Title: in.Title}, f.err
}
func (f *fakeRepo) Delete(_ context.Context, s, _ string) error { f.rec(s); return f.err }

var ident = tenant.Identity{UserID: "u", TenantID: "11111111-1111-1111-1111-111111111111"}

func TestInvalidInputNeverReachesRepository(t *testing.T) {
	r := &fakeRepo{}
	s := NewService(r, &emb.Fake{})
	if _, err := s.Create(context.Background(), ident, domain.Input{Title: " ", Content: "c"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("create: %v", err)
	}
	if _, err := s.Update(context.Background(), ident, "id", domain.Input{Title: "t", Content: ""}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("update: %v", err)
	}
	if r.calls != 0 {
		t.Errorf("repository called %d times for invalid input", r.calls)
	}
}

func TestIdentityTenantIsTheScope(t *testing.T) {
	r := &fakeRepo{}
	s := NewService(r, &emb.Fake{})
	ctx := context.Background()
	_, _ = s.Create(ctx, ident, domain.Input{Title: "t", Content: "c"})
	_, _ = s.Get(ctx, ident, "x")
	_, _ = s.List(ctx, ident, nil, nil)
	_, _ = s.Update(ctx, ident, "x", domain.Input{Title: "t", Content: "c"})
	_ = s.Delete(ctx, ident, "x")
	if r.calls != 5 || r.scope != ident.TenantID {
		t.Errorf("calls=%d scope=%q, want 5 calls scoped to %q", r.calls, r.scope, ident.TenantID)
	}
}

func TestPagination(t *testing.T) {
	ip := func(n int) *int { return &n }
	cases := []struct {
		name         string
		limit, off   *int
		wantL, wantO int
		wantErr      string
	}{
		{"defaults", nil, nil, 20, 0, ""},
		{"explicit", ip(5), ip(10), 5, 10, ""},
		{"clamped", ip(1000), nil, 100, 0, ""},
		{"limit 100", ip(100), nil, 100, 0, ""},
		{"limit zero", ip(0), nil, 0, 0, domain.MsgLimit},
		{"limit negative", ip(-1), nil, 0, 0, domain.MsgLimit},
		{"offset negative", nil, ip(-1), 0, 0, domain.MsgOffset},
	}
	for _, c := range cases {
		r := &fakeRepo{}
		res, err := NewService(r, &emb.Fake{}).List(context.Background(), ident, c.limit, c.off)
		if c.wantErr != "" {
			if !errors.Is(err, domain.ErrInvalid) || err.Error() != c.wantErr || r.calls != 0 {
				t.Errorf("%s: err=%v calls=%d", c.name, err, r.calls)
			}
			continue
		}
		if err != nil || res.Limit != c.wantL || res.Offset != c.wantO || r.limit != c.wantL || r.off != c.wantO {
			t.Errorf("%s: res=%+v repo=(%d,%d) err=%v", c.name, res, r.limit, r.off, err)
		}
	}
}
