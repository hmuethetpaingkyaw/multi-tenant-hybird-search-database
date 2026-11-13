// Package search holds the search domain: query rules, result type, repository contract.
package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DefaultLimit  = 10
	MaxLimit      = 100
	MaxQueryChars = 2000

	MsgLimit = "limit must be a positive integer"
)

// ErrInvalid marks user-correctable input errors; the error's text is safe to show to the caller.
var ErrInvalid = errors.New("invalid input")

type invalidError struct{ msg string }

func (e *invalidError) Error() string        { return e.msg }
func (e *invalidError) Is(target error) bool { return target == ErrInvalid }

func NewInvalid(format string, args ...any) error {
	return &invalidError{msg: fmt.Sprintf(format, args...)}
}

// Result is one matching document. Score is cosine similarity (higher is better).
type Result struct {
	DocumentID string
	Title      string
	Score      float64
}

// ValidateQuery trims the query and enforces the length rules.
func ValidateQuery(q string) (string, error) {
	q = strings.TrimSpace(q)
	switch {
	case q == "":
		return "", NewInvalid("query is required")
	case utf8.RuneCountInString(q) > MaxQueryChars:
		return "", NewInvalid("query must be at most %d characters", MaxQueryChars)
	}
	return q, nil
}

// NormalizeLimit applies the default (nil) and the clamp; limit < 1 is invalid.
func NormalizeLimit(limit *int) (int, error) {
	if limit == nil {
		return DefaultLimit, nil
	}
	if *limit < 1 {
		return 0, NewInvalid(MsgLimit)
	}
	return min(*limit, MaxLimit), nil
}

// Repository runs searches under the caller's tenant context (scopeTenantID).
// Implementations must not add their own tenant filters.
type Repository interface {
	Semantic(ctx context.Context, scopeTenantID string, queryVector []float32, limit int) ([]Result, error)
}
