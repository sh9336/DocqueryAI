package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"assistant/internal/db"

	"github.com/gin-gonic/gin"
)

// DebugDatabaseState returns info about documents in the database
func DebugDatabaseState(c *gin.Context) {
	log.Printf("[DEBUG] Checking database state...")

	type DocumentSummary struct {
		Filename   string `json:"filename"`
		ChunkCount int    `json:"chunk_count"`
		SampleText string `json:"sample_text"`
	}

	var documents []DocumentSummary

	// Get unique filenames and chunk counts
	rows, err := db.DB.QueryContext(c.Request.Context(), `
		SELECT filename, COUNT(*) as chunk_count
		FROM document_chunks
		GROUP BY filename
		ORDER BY filename
	`)
	if err != nil {
		log.Printf("[DEBUG] Query failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Database query failed: %v", err)})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var summary DocumentSummary
		if err := rows.Scan(&summary.Filename, &summary.ChunkCount); err != nil {
			log.Printf("[DEBUG] Scan error: %v", err)
			continue
		}

		// Get a sample chunk from this document
		var sampleText sql.NullString
		sampleErr := db.DB.QueryRowContext(c.Request.Context(), `
			SELECT content
			FROM document_chunks
			WHERE filename = $1
			LIMIT 1
		`, summary.Filename).Scan(&sampleText)

		if sampleErr == nil && sampleText.Valid {
			if len(sampleText.String) > 150 {
				summary.SampleText = sampleText.String[:150] + "..."
			} else {
				summary.SampleText = sampleText.String
			}
		}

		documents = append(documents, summary)
	}

	// Get total stats
	var totalChunks int
	statsErr := db.DB.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(*) FROM document_chunks
	`).Scan(&totalChunks)

	response := gin.H{
		"total_chunks": totalChunks,
		"unique_files": len(documents),
		"documents":    documents,
	}

	if statsErr == nil {
		log.Printf("[DEBUG] Database state: %d chunks from %d files", totalChunks, len(documents))
	}

	c.JSON(http.StatusOK, response)
}

// DebugClearDocuments removes all documents from the database (for testing)
func DebugClearDocuments(c *gin.Context) {
	log.Printf("[DEBUG] Clearing all documents from database...")

	result, err := db.DB.ExecContext(c.Request.Context(), `DELETE FROM document_chunks`)
	if err != nil {
		log.Printf("[DEBUG] Clear failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Clear failed: %v", err)})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Could not get affected rows: %v", err)})
		return
	}

	log.Printf("[DEBUG] Deleted %d chunks", rowsAffected)
	c.JSON(http.StatusOK, gin.H{"deleted": rowsAffected, "message": "All documents cleared from database"})
}
