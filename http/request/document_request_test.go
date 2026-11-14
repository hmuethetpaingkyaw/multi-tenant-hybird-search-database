package request

import (
	"errors"
	"strings"
	"testing"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
)

func wantValidation(t *testing.T, name string, err error, msg string) {
	t.Helper()
	if msg == "" {
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		return
	}
	if !errors.Is(err, apperror.ErrValidation) || err.Error() != msg {
		t.Errorf("%s: got %v, want validation error %q", name, err, msg)
	}
}

func TestDocumentRequestValidate(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name string
		req  DocumentRequest
		msg  string // empty means valid
	}{
		{"ok", DocumentRequest{"Title", "body"}, ""},
		{"empty title", DocumentRequest{"", "body"}, "title is required"},
		{"whitespace title", DocumentRequest{"   \t", "body"}, "title is required"},
		{"title 500", DocumentRequest{long(500), "body"}, ""},
		{"title 501", DocumentRequest{long(501), "body"}, "title must be at most 500 characters"},
		{"title 500 multibyte", DocumentRequest{strings.Repeat("é", 500), "body"}, ""},
		{"empty content", DocumentRequest{"t", ""}, "content is required"},
		{"whitespace content", DocumentRequest{"t", " \n "}, "content is required"},
		{"content at 1 MiB", DocumentRequest{"t", long(constants.MaxContentBytes)}, ""},
		{"content over 1 MiB", DocumentRequest{"t", long(constants.MaxContentBytes + 1)}, "content must be at most 1048576 bytes"},
		{"title is checked before content", DocumentRequest{"", ""}, "title is required"},
	}
	for _, c := range cases {
		wantValidation(t, c.name, c.req.Validate(), c.msg)
	}
}

func TestToInputTrimsTitleOnly(t *testing.T) {
	in := (&DocumentRequest{Title: "  Hello  ", Content: "  body  "}).ToInput()
	if in.Title != "Hello" || in.Content != "  body  " {
		t.Fatalf("got %+v", in)
	}
}

func str(s string) *string { return &s }

func TestDocumentListRequest(t *testing.T) {
	cases := []struct {
		name         string
		limit, off   *string
		wantL, wantO *int
		msg          string
	}{
		{"absent", nil, nil, nil, nil, ""},
		{"both", str("5"), str("10"), ptr(5), ptr(10), ""},
		{"limit zero", str("0"), nil, nil, nil, constants.MsgLimitInvalid},
		{"limit negative", str("-3"), nil, nil, nil, constants.MsgLimitInvalid},
		{"limit not int", str("abc"), nil, nil, nil, constants.MsgLimitInvalid},
		{"offset negative", nil, str("-1"), nil, nil, constants.MsgOffsetInvalid},
		{"offset not int", nil, str("x"), nil, nil, constants.MsgOffsetInvalid},
		// Parse errors win over range errors regardless of parameter order (existing contract).
		{"limit zero but offset not int", str("0"), str("x"), nil, nil, constants.MsgOffsetInvalid},
		{"limit not int and offset negative", str("x"), str("-1"), nil, nil, constants.MsgLimitInvalid},
		{"limit zero and offset negative: limit first", str("0"), str("-1"), nil, nil, constants.MsgLimitInvalid},
		{"huge limit is valid here (clamped later)", str("100000"), nil, ptr(100000), nil, ""},
	}
	for _, c := range cases {
		req, err := NewDocumentListRequest(c.limit, c.off)
		wantValidation(t, c.name, err, c.msg)
		if c.msg != "" {
			continue
		}
		if !eq(req.Limit, c.wantL) || !eq(req.Offset, c.wantO) {
			t.Errorf("%s: got %v/%v", c.name, deref(req.Limit), deref(req.Offset))
		}
	}
}

func ptr(n int) *int { return &n }
func eq(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
