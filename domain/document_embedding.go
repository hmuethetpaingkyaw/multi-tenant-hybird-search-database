package domain

// DocumentEmbedding is what is stored for one chunk of a document.
type DocumentEmbedding struct {
	Model   string
	Content string // the exact chunk text that was embedded (without model prefixes)
	Vector  []float32
}
