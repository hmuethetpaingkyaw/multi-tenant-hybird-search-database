package apperror

import (
	"errors"
	"testing"
)

func TestUnavailableMatchesSentinelAndKeepsCause(t *testing.T) {
	err := Unavailable(errors.New("connection refused"))
	if !errors.Is(err, ErrEmbeddingUnavailable) {
		t.Fatal("must match ErrEmbeddingUnavailable")
	}
	if got := err.Error(); got != "embedding service unavailable: connection refused" {
		t.Errorf("error text = %q", got)
	}
}

func TestValidationErrorIsFoundWithErrorsAs(t *testing.T) {
	var ve *ValidationError
	wrapped := errors.Join(errors.New("ctx"), NewValidationError("title must be at most %d characters", 500))
	if !errors.As(wrapped, &ve) || ve.Message != "title must be at most 500 characters" {
		t.Fatalf("got %+v", ve)
	}
}

func TestAppErrorUnwrapsCause(t *testing.T) {
	cause := errors.New("boom")
	err := &AppError{Code: 500, Message: "internal error", Err: cause}
	if !errors.Is(err, cause) {
		t.Error("AppError must unwrap to its cause")
	}
}
