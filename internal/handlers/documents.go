package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"assistant/internal/db"
	"assistant/internal/middleware"
	"assistant/internal/session"
	"github.com/gin-gonic/gin"
)

// DocumentInfo represents basic metadata for an uploaded document
type DocumentInfo struct {
	Filename  string    `json:"filename"`
	Chunks    int       `json:"chunks"`
	CreatedAt time.Time `json:"created_at"`
}

// ListDocuments retrieves a grouped list of documents chunked within the
// caller's session — never another visitor's uploads.
func ListDocuments(c *gin.Context) {
	sess := c.MustGet(middleware.SessionContextKey).(*session.Session)

	rows, err := db.DB.QueryContext(c.Request.Context(), `
		SELECT filename, COUNT(*) as chunks, MIN(created_at) as created_at
		FROM document_chunks
		WHERE session_id = $1
		GROUP BY filename
		ORDER BY created_at DESC
	`, sess.ID)
	if err != nil {
		log.Printf("[Error] Failed to query documents: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve documents from database"})
		return
	}
	defer rows.Close()

	docs := []DocumentInfo{}
	for rows.Next() {
		var doc DocumentInfo
		if err := rows.Scan(&doc.Filename, &doc.Chunks, &doc.CreatedAt); err != nil {
			log.Printf("[Error] Failed to scan document row: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse documents"})
			return
		}
		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[Error] Rows iteration error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read documents"})
		return
	}

	c.JSON(http.StatusOK, docs)
}

// DeleteDocument removes all chunks of a document owned by the caller's
// session and deletes the source file from disk. A filename belonging to a
// different session simply isn't found — sessions can't probe or delete
// each other's documents.
func DeleteDocument(c *gin.Context) {
	sess := c.MustGet(middleware.SessionContextKey).(*session.Session)

	filename := c.Param("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Filename is required"})
		return
	}

	// Sanitize to prevent path traversal
	safeFilename := filepath.Base(filename)
	if safeFilename == "." || safeFilename == "/" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filename"})
		return
	}

	// 1. Delete document chunks from DB, scoped to this session (parameterized query)
	result, err := db.DB.ExecContext(c.Request.Context(),
		"DELETE FROM document_chunks WHERE filename = $1 AND session_id = $2", safeFilename, sess.ID)
	if err != nil {
		log.Printf("[Error] Failed to delete document chunks for %s: %v", safeFilename, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove document chunks from database"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found in your session"})
		return
	}
	sessionMgr.DecrDoc(sess.ID, int(rowsAffected))

	// 2. Delete source file from this session's uploads directory
	uploadsDir := filepath.Join("./uploads", sess.ID)
	filePath := filepath.Join(uploadsDir, safeFilename)

	// Double check path traversal safety: resolve paths and verify prefix
	resolvedPath, err := filepath.Abs(filePath)
	resolvedUploads, err2 := filepath.Abs(uploadsDir)
	if err != nil || err2 != nil {
		log.Printf("[Error] Path resolution failed: %v %v", err, err2)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal path processing error"})
		return
	}

	// Prefix check with directory boundary validation
	// Enforce strict boundary to prevent partial matches bypass
	expectedPrefix := resolvedUploads + string(filepath.Separator)
	if len(resolvedPath) < len(expectedPrefix) || resolvedPath[:len(expectedPrefix)] != expectedPrefix {
		log.Printf("[Security Warning] Traversal attempt blocked: path %q is outside uploads directory %q", resolvedPath, resolvedUploads)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Access denied: invalid document path"})
		return
	}

	fileRemoved := false
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			log.Printf("[Warning] Failed to delete file %s from disk: %v", filePath, err)
			// We still succeed if DB chunks were deleted, but report warning in log.
		} else {
			fileRemoved = true
		}
	} else if os.IsNotExist(err) {
		fileRemoved = true // Treat as deleted if it doesn't exist
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Document successfully deleted",
		"filename":     safeFilename,
		"chunks":       rowsAffected,
		"file_removed": fileRemoved,
	})
}
