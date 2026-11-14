// Package request holds the HTTP request bodies/queries and their validation.
// Validation messages are part of the API contract.
package request

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
)

// DocumentRequest is the body of POST /documents and PUT /documents/:id.
type DocumentRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Validate checks the title first, then the content.
func (r *DocumentRequest) Validate() error {
	title := strings.TrimSpace(r.Title)
	switch {
	case title == "":
		return apperror.NewValidationError("title is required")
	case utf8.RuneCountInString(title) > constants.MaxTitleChars:
		return apperror.NewValidationError("title must be at most %d characters", constants.MaxTitleChars)
	case strings.TrimSpace(r.Content) == "":
		return apperror.NewValidationError("content is required")
	case len(r.Content) > constants.MaxContentBytes:
		return apperror.NewValidationError("content must be at most %d bytes", constants.MaxContentBytes)
	}
	return nil
}

// ToInput returns the domain input: the title is trimmed, the content is kept as sent.
func (r *DocumentRequest) ToInput() domain.DocumentInput {
	return domain.DocumentInput{Title: strings.TrimSpace(r.Title), Content: r.Content}
}

// DocumentListRequest is the query of GET /documents. A nil field means "not provided".
type DocumentListRequest struct {
	Limit  *int
	Offset *int
}

// NewDocumentListRequest parses the raw query values (nil = parameter absent) and validates them.
// Order matters for the contract: both values are parsed before either range is checked.
func NewDocumentListRequest(rawLimit, rawOffset *string) (*DocumentListRequest, error) {
	req := &DocumentListRequest{}
	if rawLimit != nil {
		n, err := strconv.Atoi(*rawLimit)
		if err != nil {
			return nil, apperror.NewValidationError("%s", constants.MsgLimitInvalid)
		}
		req.Limit = &n
	}
	if rawOffset != nil {
		n, err := strconv.Atoi(*rawOffset)
		if err != nil {
			return nil, apperror.NewValidationError("%s", constants.MsgOffsetInvalid)
		}
		req.Offset = &n
	}
	if req.Limit != nil && *req.Limit < 1 {
		return nil, apperror.NewValidationError("%s", constants.MsgLimitInvalid)
	}
	if req.Offset != nil && *req.Offset < 0 {
		return nil, apperror.NewValidationError("%s", constants.MsgOffsetInvalid)
	}
	return req, nil
}
