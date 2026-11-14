package request

import (
	"strings"
	"testing"

	"multi-tenant-hybrid-search/constants"
)

func TestSearchRequestValidate(t *testing.T) {
	zero, neg, ten := 0, -3, 10
	cases := []struct {
		name string
		req  SearchRequest
		msg  string
	}{
		{"ok", SearchRequest{Query: "recover account"}, ""},
		{"ok with limit", SearchRequest{Query: "x", Limit: &ten}, ""},
		{"empty", SearchRequest{}, "query is required"},
		{"blank", SearchRequest{Query: "  \t "}, "query is required"},
		{"2000 chars", SearchRequest{Query: strings.Repeat("a", 2000)}, ""},
		{"2001 chars", SearchRequest{Query: strings.Repeat("a", 2001)}, "query must be at most 2000 characters"},
		{"limit 0", SearchRequest{Query: "x", Limit: &zero}, constants.MsgLimitInvalid},
		{"limit negative", SearchRequest{Query: "x", Limit: &neg}, constants.MsgLimitInvalid},
		{"query is checked before limit", SearchRequest{Query: "", Limit: &zero}, "query is required"},
	}
	for _, c := range cases {
		wantValidation(t, c.name, c.req.Validate(), c.msg)
	}
}

func TestQueryTextIsTrimmed(t *testing.T) {
	if got := (&SearchRequest{Query: "  hello  "}).QueryText(); got != "hello" {
		t.Errorf("got %q", got)
	}
}
