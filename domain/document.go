package domain

import "time"

type Document struct {
	ID        string
	Title     string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DocumentSummary is a document without its content, for listings.
type DocumentSummary struct {
	ID        string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DocumentInput is the user-supplied part of a document.
type DocumentInput struct {
	Title   string
	Content string
}
