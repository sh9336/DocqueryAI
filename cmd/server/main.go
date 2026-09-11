package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"assistant/internal/config"
	"assistant/internal/db"
	"assistant/internal/handlers"
	"assistant/internal/session"

	"assistant/internal/middleware"
	"assistant/internal/services"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize database
	if err := db.InitDB(cfg.DatabaseURL); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	defer db.CloseDB()

	// Initialize OpenAI service (using Cohere)
	if cfg.CohereAPIKey == "" {
		log.Println("WARNING: COHERE_API_KEY is not set. Embedding and ask endpoints will fail.")
	}
	services.InitOpenAI(cfg.CohereAPIKey)
	services.InitRateLimits(cfg.CohereEmbedRPM, cfg.CohereChatRPM)

	// Session lease manager — a session's data (chunks + uploaded files) is
	// deleted whenever its lease ends, whether by explicit release or expiry.
	cleanupSession := func(sessionID string) {
		if _, err := db.DB.Exec("DELETE FROM document_chunks WHERE session_id = $1", sessionID); err != nil {
			log.Printf("[SESSION] cleanup: failed to delete chunks for %s: %v", sessionID, err)
		}
		dir := filepath.Join("./uploads", sessionID)
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("[SESSION] cleanup: failed to remove uploads dir for %s: %v", sessionID, err)
		}
	}
	sessionMgr := session.NewManager(session.Limits{
		MaxSessions:      cfg.MaxSessions,
		MaxSessionsPerIP: cfg.MaxSessionsPerIP,
		HeartbeatTimeout: cfg.SessionHeartbeatTimeout,
		MaxLifetime:      cfg.SessionMaxLifetime,
		MaxDocs:          cfg.MaxDocsPerSession,
		MaxChunks:        cfg.MaxChunksPerSession,
		MaxQuestions:     cfg.MaxQuestionsPerSession,
	}, cleanupSession)
	sessionMgr.StartSweeper(30 * time.Second)
	handlers.InitSession(sessionMgr, cfg)

	// Set up Gin router
	router := gin.Default()

	// CORS — must be registered before any route so OPTIONS preflight is handled
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Session lease endpoints. Creation is public (that's how a lease is
	// acquired); heartbeat/release manage their own missing-session cases
	// internally, so only /session/status needs the RequireSession guard.
	router.POST("/session", handlers.CreateSession)
	router.POST("/session/heartbeat", handlers.Heartbeat)
	router.POST("/session/release", handlers.ReleaseSession)
	router.GET("/session/status", middleware.RequireSession(sessionMgr), handlers.SessionStatus)

	// RAG endpoints — all scoped to the caller's session
	router.POST("/upload", middleware.RequireSession(sessionMgr), handlers.UploadPDF)
	router.POST("/search", middleware.RequireSession(sessionMgr), handlers.Search)
	router.POST("/ask", middleware.RequireSession(sessionMgr), handlers.Ask)
	router.GET("/documents", middleware.RequireSession(sessionMgr), handlers.ListDocuments)
	router.DELETE("/documents/:filename", middleware.RequireSession(sessionMgr), handlers.DeleteDocument)

	// Debug endpoints (for troubleshooting) — only registered at all when a
	// DEBUG_TOKEN is configured, so they're absent (not just unauthorized)
	// on a default production deployment.
	if cfg.DebugToken != "" {
		debugAuth := middleware.RequireDebugToken(cfg.DebugToken)
		router.GET("/debug/state", debugAuth, handlers.DebugDatabaseState)
		router.POST("/debug/clear", debugAuth, handlers.DebugClearDocuments)
	} else {
		log.Println("DEBUG_TOKEN not set — /debug/* endpoints are disabled")
	}

	// Start server
	log.Printf("Starting server on port %s...", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
