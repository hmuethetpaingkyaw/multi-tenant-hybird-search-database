package usecase

import (
	"context"
	"errors"
	"testing"

	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/repository/mock"
)

func TestSearchLimitDefaultsClampAndScope(t *testing.T) {
	big := 1000
	for _, c := range []struct {
		limit *int
		want  int
	}{{nil, 10}, {&big, 100}} {
		r := &mock_repository.MockSearchPort{}
		res, err := NewSearchUsecase(r, &mock_ollama.Fake{}).Semantic(context.Background(), ident, "recover account", c.limit)
		if err != nil || r.Limit != c.want || r.Scope != ident.TenantID || len(r.Vector) != constants.EmbeddingDimensions {
			t.Errorf("limit %v: repo limit=%d scope=%q vec=%d err=%v", c.limit, r.Limit, r.Scope, len(r.Vector), err)
		}
		if res == nil {
			t.Error("result must be a non-nil empty slice")
		}
	}
}

func TestSearchRejectsLimitBelowOneWithoutTouchingDependencies(t *testing.T) {
	zero := 0
	r, f := &mock_repository.MockSearchPort{}, &mock_ollama.Fake{}
	_, err := NewSearchUsecase(r, f).Semantic(context.Background(), ident, "q", &zero)
	if !errors.Is(err, apperror.ErrValidation) || err.Error() != constants.MsgLimitInvalid {
		t.Errorf("err = %v", err)
	}
	if r.Calls != 0 || f.Calls != 0 {
		t.Errorf("dependencies called (repo %d, embedder %d)", r.Calls, f.Calls)
	}
}

func TestSearchEmbeddingFailureIsUnavailableAndSkipsDatabase(t *testing.T) {
	r := &mock_repository.MockSearchPort{}
	_, err := NewSearchUsecase(r, &mock_ollama.Fake{Err: errors.New("down")}).Semantic(context.Background(), ident, "q", nil)
	if !errors.Is(err, apperror.ErrEmbeddingUnavailable) {
		t.Errorf("err = %v", err)
	}
	if r.Calls != 0 {
		t.Error("repository must not be called when embedding fails")
	}
}

func TestSearchPassesResultsThrough(t *testing.T) {
	r := &mock_repository.MockSearchPort{Results: []domain.SearchResult{{DocumentID: "d", Title: "t", Score: 0.9}}}
	res, err := NewSearchUsecase(r, &mock_ollama.Fake{}).Semantic(context.Background(), ident, "q", nil)
	if err != nil || len(res) != 1 || res[0].DocumentID != "d" {
		t.Errorf("res=%+v err=%v", res, err)
	}
}
