package request

import (
	"strings"
	"unicode/utf8"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
)

// SearchRequest is the body of POST /search/semantic.
type SearchRequest struct {
	Query string `json:"query"`
	Limit *int   `json:"limit"`
}

// Validate checks the query first, then the limit.
func (r *SearchRequest) Validate() error {
	q := strings.TrimSpace(r.Query)
	switch {
	case q == "":
		return apperror.NewValidationError("query is required")
	case utf8.RuneCountInString(q) > constants.MaxQueryChars:
		return apperror.NewValidationError("query must be at most %d characters", constants.MaxQueryChars)
	}
	if r.Limit != nil && *r.Limit < 1 {
		return apperror.NewValidationError("%s", constants.MsgLimitInvalid)
	}
	return nil
}

// QueryText returns the trimmed query.
func (r *SearchRequest) QueryText() string { return strings.TrimSpace(r.Query) }
