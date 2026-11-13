// Package document holds the document domain types, validation and repository contract.
package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxTitleChars   = 500
	MaxContentBytes = 1 << 20 // 1 MiB
)

var (
	// ErrNotFound means the document does not exist or is not visible to the caller's tenant.
	// The two cases are deliberately indistinguishable.
	ErrNotFound = errors.New("not found")
	// ErrInvalid marks user-correctable input errors; the error's text is safe to show to the caller.
	ErrInvalid = errors.New("invalid input")
)

type invalidError struct{ msg string }

func (e *invalidError) Error() string        { return e.msg }
func (e *invalidError) Is(target error) bool { return target == ErrInvalid }

// NewInvalid returns an error matching ErrInvalid whose text is the user-facing message.
func NewInvalid(format string, args ...any) error {
	return &invalidError{msg: fmt.Sprintf(format, args...)}
}

type Document struct {
	ID        string
	Title     string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Summary is a document without its content, for listings.
type Summary struct {
	ID        string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Input is the user-supplied part of a document.
type Input struct {
	Title   string
	Content string
}

const (
	DefaultLimit = 20
	MaxLimit     = 100

	MsgLimit  = "limit must be a positive integer"
	MsgOffset = "offset must be a non-negative integer"
)

// NormalizePage applies pagination defaults and bounds. nil means "not provided".
// limit < 1 or offset < 0 is invalid; limit above MaxLimit is clamped.
func NormalizePage(limit, offset *int) (int, int, error) {
	l, o := DefaultLimit, 0
	if limit != nil {
		if *limit < 1 {
			return 0, 0, NewInvalid(MsgLimit)
		}
		l = min(*limit, MaxLimit)
	}
	if offset != nil {
		if *offset < 0 {
			return 0, 0, NewInvalid(MsgOffset)
		}
		o = *offset
	}
	return l, o, nil
}

// Validate returns the input with the title trimmed, or an ErrInvalid error with a user-safe message.
func (in Input) Validate() (Input, error) {
	title := strings.TrimSpace(in.Title)
	switch {
	case title == "":
		return Input{}, NewInvalid("title is required")
	case utf8.RuneCountInString(title) > MaxTitleChars:
		return Input{}, NewInvalid("title must be at most %d characters", MaxTitleChars)
	case strings.TrimSpace(in.Content) == "":
		return Input{}, NewInvalid("content is required")
	case len(in.Content) > MaxContentBytes:
		return Input{}, NewInvalid("content must be at most %d bytes", MaxContentBytes)
	}
	return Input{Title: title, Content: in.Content}, nil
}

// Repository persists documents. scopeTenantID is the RLS context every call runs under;
// implementations must not add their own tenant filters.
type Repository interface {
	Create(ctx context.Context, scopeTenantID string, in Input) (Document, error)
	Get(ctx context.Context, scopeTenantID, id string) (Document, error)
	List(ctx context.Context, scopeTenantID string, limit, offset int) ([]Summary, error)
	Update(ctx context.Context, scopeTenantID, id string, in Input) (Document, error)
	Delete(ctx context.Context, scopeTenantID, id string) error
}
