package models

import "time"

// DocumentChunk represents a chunk of text with its vector embedding.
type DocumentChunk struct {
	ID          int       `json:"id"`
	Filename    string    `json:"filename"`
	Content     string    `json:"content"`
	Embedding   []float32 `json:"embedding,omitempty"`
	PageNumber  int       `json:"page_number,omitempty"`
	Section     string    `json:"section,omitempty"`
	ChunkIndex  int       `json:"chunk_index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SearchRequest represents the body of a search request.
type SearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit,omitempty"`
}

// AskRequest represents the body of an ask request. ContinueAnswer, when
// set, is the prior (truncated) answer text the client wants continued —
// the caller resends the original Question alongside it so retrieval can
// be redone and the model can pick up where it left off.
//
// The response to /ask is not a fixed JSON shape — it's a text/event-stream
// of {"type": "delta"|"done"|"error", ...} events (see ask.go's startSSE),
// so there's no corresponding AskResponse struct.
type AskRequest struct {
	Question       string `json:"question" binding:"required"`
	Limit          int    `json:"limit,omitempty"`
	ContinueAnswer string `json:"continue_answer,omitempty"`
}
