package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"assistant/internal/db"
	"assistant/internal/middleware"
	"assistant/internal/models"
	"assistant/internal/services"
	"assistant/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
)

// startSSE marks the response as a Server-Sent Events stream and returns a
// function that writes one JSON-encoded event, flushing immediately so the
// frontend sees each chunk as it's written rather than buffered. Must only
// be called once no earlier error response could still be needed — once
// this runs, the HTTP status is committed to 200 and any later failure has
// to be reported as an "error" event, not a status code.
func startSSE(c *gin.Context) func(v any) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // disable nginx response buffering, if fronted by one
	c.Writer.WriteHeader(http.StatusOK)
	flusher, canFlush := c.Writer.(http.Flusher)

	return func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		if canFlush {
			flusher.Flush()
		}
	}
}

func Ask(c *gin.Context) {
	sess := c.MustGet(middleware.SessionContextKey).(*session.Session)

	var req models.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ASK] Invalid request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate and trim question
	question := strings.TrimSpace(req.Question)
	if question == "" {
		log.Printf("[ASK] ❌ Question is empty")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Question cannot be empty"})
		return
	}
	if len(question) > appConfig.MaxQuestionLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Question is too long (max %d characters)", appConfig.MaxQuestionLength)})
		return
	}

	if err := sessionMgr.TryStartChat(sess.ID); err != nil {
		status := http.StatusConflict
		msg := "Another question is already being answered for this session."
		if errors.Is(err, session.ErrNotFound) {
			status = http.StatusGone
			msg = "Session expired."
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}
	defer sessionMgr.FinishChat(sess.ID)

	if err := sessionMgr.CheckAndIncrQuestions(sess.ID); err != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": fmt.Sprintf("You've reached the %d-question limit for this demo session.", sessionMgr.Limits().MaxQuestions)})
		return
	}

	log.Printf("[ASK] [%s] Question: %q", sess.ID, question)

	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	log.Printf("[ASK] Generating embedding for question...")
	embeddings, err := services.GenerateEmbeddings(c.Request.Context(), []string{req.Question}, "search_query")
	if err != nil {
		log.Printf("[ASK] ❌ Embedding generation failed: %v", err)
		if errors.Is(err, services.ErrRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error(), "stage": "embeddings"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate embedding: %v", err), "stage": "embeddings"})
		return
	}
	log.Printf("[ASK] ✓ Generated embedding successfully")

	if len(embeddings) == 0 {
		log.Printf("[ASK] Empty embedding returned!")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Empty embedding returned"})
		return
	}
	queryEmbedding := pgvector.NewVector(embeddings[0])

	log.Printf("[ASK] Searching database for %d most relevant chunks...", limit)
	rows, err := db.DB.QueryContext(c.Request.Context(), `
		SELECT id, filename, content, page_number, section, chunk_index
		FROM document_chunks
		WHERE session_id = $1
		ORDER BY embedding <-> $2
		LIMIT $3
	`, sess.ID, queryEmbedding, limit)
	if err != nil {
		log.Printf("[ASK] Database retrieval failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Retrieval failed: %v", err)})
		return
	}
	defer rows.Close()

	var sources []models.DocumentChunk
	var contextTexts []string
	for rows.Next() {
		var chunk models.DocumentChunk
		var pageNumber sql.NullInt32
		var section sql.NullString
		if err := rows.Scan(&chunk.ID, &chunk.Filename, &chunk.Content, &pageNumber, &section, &chunk.ChunkIndex); err != nil {
			log.Printf("[ASK] Failed to parse result row: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to parse results: %v", err)})
			return
		}
		if pageNumber.Valid {
			chunk.PageNumber = int(pageNumber.Int32)
		}
		if section.Valid {
			chunk.Section = section.String
		}
		sources = append(sources, chunk)
		contextTexts = append(contextTexts, chunk.Content)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[ASK] Error reading result rows: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read results: %v", err)})
		return
	}

	log.Printf("[ASK] Found %d relevant chunks (%.0f%% of database if loaded)", len(sources), float64(len(sources))*100/float64(limit))

	if len(sources) == 0 {
		log.Printf("[ASK] ⚠️  No chunks found in database - check if documents have been uploaded with embeddings")
	}

	contextStr := strings.Join(contextTexts, "\n\n---\n\n")
	log.Printf("[ASK] Context: %d chars from %d chunks", len(contextStr), len(sources))

	log.Printf("[ASK] Generating answer with question: %q", question)
	stream, err := services.OpenAnswerStream(c.Request.Context(), contextStr, question, req.ContinueAnswer)
	if err != nil {
		if errors.Is(err, services.ErrNoContext) {
			log.Printf("[ASK] ⚠️ No context available for question: %q", question)
			writeEvent := startSSE(c)
			writeEvent(gin.H{"type": "delta", "text": "I don't know - no documents were found to answer your question."})
			writeEvent(gin.H{"type": "done", "sources": sources, "truncated": false})
			return
		}
		log.Printf("[ASK] ❌ Answer generation failed: %v", err)
		if errors.Is(err, services.ErrRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error(), "stage": "answer"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate answer: %v", err), "stage": "answer"})
		return
	}

	// From here on the response is committed to SSE (200, streaming) — any
	// failure in PumpAnswerStream can only be reported as an "error" event.
	writeEvent := startSSE(c)
	_, truncated, streamErr := services.PumpAnswerStream(stream, func(delta string) {
		writeEvent(gin.H{"type": "delta", "text": delta})
	})
	if streamErr != nil {
		log.Printf("[ASK] ❌ Answer stream interrupted: %v", streamErr)
		writeEvent(gin.H{"type": "error", "message": "The connection to the AI service was interrupted. Please try again."})
		return
	}

	log.Printf("[ASK] ✓ Answer stream complete")
	writeEvent(gin.H{"type": "done", "sources": sources, "truncated": truncated})
}
