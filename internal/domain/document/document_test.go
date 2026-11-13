package document

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name    string
		in      Input
		wantErr string // empty means valid
	}{
		{"ok", Input{"Title", "body"}, ""},
		{"empty title", Input{"", "body"}, "title is required"},
		{"whitespace title", Input{"   \t", "body"}, "title is required"},
		{"title 500", Input{long(500), "body"}, ""},
		{"title 501", Input{long(501), "body"}, "title must be at most 500 characters"},
		{"title 500 multibyte", Input{strings.Repeat("é", 500), "body"}, ""},
		{"empty content", Input{"t", ""}, "content is required"},
		{"whitespace content", Input{"t", " \n "}, "content is required"},
		{"content at 1 MiB", Input{"t", long(MaxContentBytes)}, ""},
		{"content over 1 MiB", Input{"t", long(MaxContentBytes + 1)}, "content must be at most 1048576 bytes"},
	}
	for _, c := range cases {
		_, err := c.in.Validate()
		if c.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalid) || err.Error() != c.wantErr {
			t.Errorf("%s: got %v, want ErrInvalid %q", c.name, err, c.wantErr)
		}
	}
}

func TestValidateTrimsTitleOnly(t *testing.T) {
	got, err := Input{"  Hello  ", "  body  "}.Validate()
	if err != nil || got.Title != "Hello" || got.Content != "  body  " {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
