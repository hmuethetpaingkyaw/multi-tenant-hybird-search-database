package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/repository/mock"
)

var ident = domain.Identity{UserID: "u", TenantID: "11111111-1111-1111-1111-111111111111"}

func newDoc(r *mock_repository.MockDocumentPort, e *mock_ollama.Fake) *DocumentUsecase {
	return NewDocumentUsecase(r, e)
}

func TestIdentityTenantIsTheScope(t *testing.T) {
	r := &mock_repository.MockDocumentPort{}
	u := newDoc(r, &mock_ollama.Fake{})
	ctx := context.Background()
	in := domain.DocumentInput{Title: "t", Content: "c"}
	_, _ = u.Create(ctx, ident, in)
	_, _ = u.Get(ctx, ident, "x")
	_, _ = u.List(ctx, ident, nil, nil)
	_, _ = u.Update(ctx, ident, "x", in)
	_ = u.Delete(ctx, ident, "x")
	if r.Calls != 5 || r.Scope != ident.TenantID {
		t.Errorf("calls=%d scope=%q, want 5 calls scoped to %q", r.Calls, r.Scope, ident.TenantID)
	}
}

func TestEmbeddingFailureNeverReachesRepository(t *testing.T) {
	for name, cause := range map[string]error{
		"plain error":    errors.New("connection refused"),
		"already tagged": apperror.Unavailable(errors.New("down")),
	} {
		r := &mock_repository.MockDocumentPort{}
		u := newDoc(r, &mock_ollama.Fake{Err: cause})
		in := domain.DocumentInput{Title: "t", Content: "c"}
		if _, err := u.Create(context.Background(), ident, in); !errors.Is(err, apperror.ErrEmbeddingUnavailable) {
			t.Errorf("%s create: %v", name, err)
		}
		if _, err := u.Update(context.Background(), ident, "x", in); !errors.Is(err, apperror.ErrEmbeddingUnavailable) {
			t.Errorf("%s update: %v", name, err)
		}
		if r.Calls != 0 {
			t.Errorf("%s: repository called %d times after embedding failure", name, r.Calls)
		}
	}
}

func TestEmbedsTitlePlusContentTruncatedToLimit(t *testing.T) {
	long := strings.Repeat("é", constants.MaxEmbedChars+500) // multibyte: counts characters, not bytes
	chunk := buildChunk(domain.DocumentInput{Title: "T", Content: long})
	if n := len([]rune(chunk)); n != constants.MaxEmbedChars {
		t.Fatalf("chunk length = %d, want %d", n, constants.MaxEmbedChars)
	}
	if got := buildChunk(domain.DocumentInput{Title: "T", Content: "body"}); got != "T\n\nbody" {
		t.Errorf("chunk = %q", got)
	}
}

func TestListPaging(t *testing.T) {
	ip := func(n int) *int { return &n }
	cases := []struct {
		name         string
		limit, off   *int
		wantL, wantO int
		invalid      string
	}{
		{"defaults", nil, nil, 20, 0, ""},
		{"explicit", ip(5), ip(10), 5, 10, ""},
		{"clamped", ip(1000), nil, 100, 0, ""},
		{"limit 100", ip(100), nil, 100, 0, ""},
		{"limit zero", ip(0), nil, 0, 0, constants.MsgLimitInvalid},
		{"limit negative", ip(-1), nil, 0, 0, constants.MsgLimitInvalid},
		{"offset negative", nil, ip(-1), 0, 0, constants.MsgOffsetInvalid},
	}
	for _, c := range cases {
		r := &mock_repository.MockDocumentPort{}
		res, err := newDoc(r, &mock_ollama.Fake{}).List(context.Background(), ident, c.limit, c.off)
		if c.invalid != "" {
			if !errors.Is(err, apperror.ErrValidation) || err.Error() != c.invalid || r.Calls != 0 {
				t.Errorf("%s: err=%v calls=%d", c.name, err, r.Calls)
			}
			continue
		}
		if err != nil || res.Limit != c.wantL || res.Offset != c.wantO || r.Limit != c.wantL || r.Offset != c.wantO {
			t.Errorf("%s: res=%+v repo=(%d,%d) err=%v", c.name, res, r.Limit, r.Offset, err)
		}
	}
}

func TestRepositoryErrorsPassThrough(t *testing.T) {
	r := &mock_repository.MockDocumentPort{Err: apperror.ErrNotFound}
	u := newDoc(r, &mock_ollama.Fake{})
	if _, err := u.Get(context.Background(), ident, "x"); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("get: %v", err)
	}
	if err := u.Delete(context.Background(), ident, "x"); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("delete: %v", err)
	}
}
