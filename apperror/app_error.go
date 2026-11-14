// Package apperror defines the application's error types. Layers below the HTTP controllers return
// these (or wrap them); the single error middleware turns them into HTTP responses.
package apperror

import (
	"errors"
	"fmt"
	"net/http"

	"multi-tenant-hybrid-search/constants"
)

var (
	// ErrNotFound means the row does not exist or is not visible to the caller's tenant.
	// The two cases are deliberately indistinguishable.
	ErrNotFound = errors.New("not found")
	// ErrEmbeddingUnavailable means the embedding service failed (down, timeout, bad response,
	// wrong model or dimension). The wrapped cause is for logs only.
	ErrEmbeddingUnavailable = errors.New("embedding service unavailable")
)

// Unavailable wraps cause so that errors.Is(err, ErrEmbeddingUnavailable) holds and the cause stays readable in logs.
func Unavailable(cause error) error { return fmt.Errorf("%w: %w", ErrEmbeddingUnavailable, cause) }

// ErrValidation matches every *ValidationError (use errors.Is).
var ErrValidation = errors.New("validation error")

// ValidationError is a user-correctable input error; its text is safe to show to the caller.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string        { return e.Message }
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

// NewValidationError builds a ValidationError from a format string.
func NewValidationError(format string, args ...any) *ValidationError {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// AppError is an error with an explicit HTTP status and a user-facing message.
// Header carries extra response headers (for example WWW-Authenticate on 401).
type AppError struct {
	Code    int
	Message string
	Header  map[string]string
	Err     error // cause, for logs only
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%d %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

// Constructors for the explicit HTTP errors controllers and middleware return.

func Unauthorized() *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: constants.MsgUnauthorized, Header: map[string]string{"WWW-Authenticate": "Bearer"}}
}

func BadRequest(message string, cause error) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: message, Err: cause}
}

func PayloadTooLarge(cause error) *AppError {
	return &AppError{Code: http.StatusRequestEntityTooLarge, Message: constants.MsgBodyTooLarge, Err: cause}
}
