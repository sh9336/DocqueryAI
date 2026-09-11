package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"assistant/internal/db"
	"assistant/internal/middleware"
	"assistant/internal/models"
	"assistant/internal/services"
	"assistant/internal/session"
	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
)

func Search(c *gin.Context) {
	sess := c.MustGet(middleware.SessionContextKey).(*session.Session)

	var req models.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	embeddings, err := services.GenerateEmbeddings(c.Request.Context(), []string{req.Query}, "search_query")
	if err != nil {
		if errors.Is(err, services.ErrRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate embedding: %v", err)})
		return
	}
	if len(embeddings) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Empty embedding returned"})
		return
	}
	queryEmbedding := pgvector.NewVector(embeddings[0])

	rows, err := db.DB.QueryContext(c.Request.Context(), `
		SELECT id, filename, content, page_number, section, chunk_index
		FROM document_chunks
		WHERE session_id = $1
		ORDER BY embedding <-> $2
		LIMIT $3
	`, sess.ID, queryEmbedding, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Search failed: %v", err)})
		return
	}
	defer rows.Close()

	var results []models.DocumentChunk
	for rows.Next() {
		var chunk models.DocumentChunk
		var pageNumber sql.NullInt32
		var section sql.NullString
		if err := rows.Scan(&chunk.ID, &chunk.Filename, &chunk.Content, &pageNumber, &section, &chunk.ChunkIndex); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to parse results: %v", err)})
			return
		}
		if pageNumber.Valid {
			chunk.PageNumber = int(pageNumber.Int32)
		}
		if section.Valid {
			chunk.Section = section.String
		}
		results = append(results, chunk)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read results: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}
