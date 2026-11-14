package constants

// User-facing error texts. These are part of the HTTP contract.
const (
	MsgUnauthorized         = "unauthorized"
	MsgNotFound             = "not found"
	MsgInternalError        = "internal error"
	MsgBodyTooLarge         = "request body too large"
	MsgInvalidBody          = "invalid request body"
	MsgInvalidDocumentID    = "invalid document id"
	MsgEmbeddingUnavailable = "embedding service unavailable"
)

// Validation messages for paging parameters (shared by the document list and search requests).
const (
	MsgLimitInvalid  = "limit must be a positive integer"
	MsgOffsetInvalid = "offset must be a non-negative integer"
)
