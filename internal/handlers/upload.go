package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"assistant/internal/db"
	"assistant/internal/middleware"
	"assistant/internal/services"
	"assistant/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
)

func UploadPDF(c *gin.Context) {
	sess := c.MustGet(middleware.SessionContextKey).(*session.Session)

	if err := sessionMgr.TryStartUpload(sess.ID); err != nil {
		status := http.StatusConflict
		msg := "An upload is already in progress for this session."
		if errors.Is(err, session.ErrNotFound) {
			status = http.StatusGone
			msg = "Session expired."
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}
	defer sessionMgr.FinishUpload(sess.ID)

	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("[UPLOAD] Missing file: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}

	log.Printf("[UPLOAD] [%s] Starting upload of: %s (size: %d bytes)", sess.ID, file.Filename, file.Size)

	maxUploadSize := int64(appConfig.MaxFileSizeMB) << 20
	if file.Size > maxUploadSize {
		log.Printf("[UPLOAD] File too large: %d bytes (max: %d)", file.Size, maxUploadSize)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("File too large (max %d MB)", appConfig.MaxFileSizeMB)})
		return
	}

	// Validate PDF magic bytes before saving
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
		return
	}
	magic := make([]byte, 4)
	_, err = io.ReadFull(src, magic)
	src.Close()
	if err != nil || !bytes.Equal(magic, []byte("%PDF")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File must be a valid PDF"})
		return
	}

	// Each session gets its own upload directory so filenames can't collide
	// across sessions and cleanup is a single os.RemoveAll on session end.
	uploadsDir := filepath.Join("./uploads", sess.ID)
	if err := os.MkdirAll(uploadsDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create uploads directory"})
		return
	}
	// Strip directory components to prevent path traversal
	safeFilename := filepath.Base(file.Filename)
	dst := filepath.Join(uploadsDir, safeFilename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	text, pageCount, err := services.ExtractTextFromPDF(dst)
	if err != nil {
		log.Printf("[UPLOAD] Failed to extract text from PDF: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to extract text: %v", err)})
		return
	}

	if pageCount > appConfig.MaxPagesPerDoc {
		log.Printf("[UPLOAD] Document has too many pages: %d (max %d)", pageCount, appConfig.MaxPagesPerDoc)
		os.Remove(dst)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Document has too many pages (max %d)", appConfig.MaxPagesPerDoc)})
		return
	}

	log.Printf("[UPLOAD] Extracted %d characters, %d pages from PDF", len(text), pageCount)

	chunks := services.ChunkText(text, 1000, 200)
	if len(chunks) == 0 {
		log.Printf("[UPLOAD] No text could be chunked from: %s", file.Filename)
		os.Remove(dst)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No text could be extracted or chunked"})
		return
	}

	log.Printf("[UPLOAD] Created %d chunks from PDF", len(chunks))

	// Reserve this session's doc/chunk quota before spending any Cohere
	// calls on it. Rolled back via DecrDoc if anything below fails.
	if err := sessionMgr.CheckAndIncrDocs(sess.ID, len(chunks)); err != nil {
		os.Remove(dst)
		status := http.StatusBadRequest
		msg := "Document limit reached for this session."
		if errors.Is(err, session.ErrChunkLimit) {
			msg = "This document would exceed your session's total chunk limit."
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}
	committed := false
	defer func() {
		if !committed {
			sessionMgr.DecrDoc(sess.ID, len(chunks))
		}
	}()

	log.Printf("[UPLOAD] Generating embeddings for %d chunks...", len(chunks))
	embeddings, err := services.GenerateEmbeddings(c.Request.Context(), chunks, "search_document")
	if err != nil {
		log.Printf("[UPLOAD] ❌ Embedding generation failed: %v", err)
		os.Remove(dst)
		if errors.Is(err, services.ErrRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error(), "stage": "embeddings"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate embeddings: %v", err), "stage": "embeddings"})
		return
	}
	log.Printf("[UPLOAD] ✓ Generated %d embeddings", len(embeddings))

	if len(embeddings) != len(chunks) {
		log.Printf("[UPLOAD] Embedding count mismatch: got %d, expected %d", len(embeddings), len(chunks))
		os.Remove(dst)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Embedding count mismatch"})
		return
	}

	// Wrap all inserts in a transaction so a mid-loop failure leaves no orphan chunks
	tx, err := db.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		os.Remove(dst)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to begin transaction"})
		return
	}
	defer tx.Rollback()

	for i, chunkText := range chunks {
		embedding := pgvector.NewVector(embeddings[i])
		_, err := tx.ExecContext(c.Request.Context(),
			"INSERT INTO document_chunks (session_id, filename, content, embedding, chunk_index) VALUES ($1, $2, $3, $4, $5)",
			sess.ID, safeFilename, chunkText, embedding, i)
		if err != nil {
			log.Printf("[UPLOAD] Failed to store chunk %d in DB: %v", i, err)
			os.Remove(dst)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to store chunk %d: %v", i, err), "stage": "database"})
			return
		}
	}

	log.Printf("[UPLOAD] Stored %d chunks, committing transaction...", len(chunks))
	if err := tx.Commit(); err != nil {
		log.Printf("[UPLOAD] Failed to commit transaction: %v", err)
		os.Remove(dst)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit chunks"})
		return
	}

	committed = true
	c.JSON(http.StatusOK, gin.H{"message": "File uploaded and processed successfully", "chunks": len(chunks)})
}
