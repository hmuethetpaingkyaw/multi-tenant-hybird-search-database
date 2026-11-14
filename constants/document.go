// Package constants holds limits and fixed texts shared across layers.
package constants

const (
	MaxTitleChars   = 500
	MaxContentBytes = 1 << 20 // 1 MiB

	DefaultListLimit = 20
	MaxListLimit     = 100

	// MaxEmbedChars bounds the text that is embedded: one chunk per document, so only the first
	// MaxEmbedChars characters of title + content influence semantic search. Real chunking comes later.
	MaxEmbedChars = 8000
)

const (
	// MaxDocumentBodyBytes caps document request bodies: the content limit plus room for JSON framing.
	MaxDocumentBodyBytes = MaxContentBytes + 64<<10
	// MaxSearchBodyBytes caps search request bodies (a query is at most MaxQueryChars characters).
	MaxSearchBodyBytes = 16 << 10
)
