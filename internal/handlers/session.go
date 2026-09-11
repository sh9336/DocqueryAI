package handlers

import (
	"net/http"

	"assistant/internal/config"
	"assistant/internal/middleware"
	"assistant/internal/session"

	"github.com/gin-gonic/gin"
)

var (
	sessionMgr *session.Manager
	appConfig  *config.Config
)

// InitSession wires the session manager and config into the handlers
// package, mirroring services.InitOpenAI's package-level init pattern.
func InitSession(mgr *session.Manager, cfg *config.Config) {
	sessionMgr = mgr
	appConfig = cfg
}

func limitsJSON(l session.Limits) gin.H {
	return gin.H{
		"max_sessions":         l.MaxSessions,
		"heartbeat_seconds":    30, // fixed client cadence — well under the 2 min inactivity timeout
		"timeout_seconds":      int(l.HeartbeatTimeout.Seconds()),
		"max_lifetime_seconds": int(l.MaxLifetime.Seconds()),
		"max_documents":        l.MaxDocs,
		"max_chunks":           l.MaxChunks,
		"max_questions":        l.MaxQuestions,
		"max_file_size_mb":     appConfig.MaxFileSizeMB,
		"max_pages_per_doc":    appConfig.MaxPagesPerDoc,
		"max_question_length":  appConfig.MaxQuestionLength,
	}
}

// CreateSession acquires a new lease, subject to the global and per-IP
// concurrency caps, and sets the session cookie.
func CreateSession(c *gin.Context) {
	// Reuse an active lease when the browser reconnects. This prevents refreshes
	// and transient retries from consuming another per-IP session slot.
	if id, err := c.Cookie(middleware.SessionCookieName); err == nil && id != "" {
		if _, ok := sessionMgr.Get(id); ok {
			c.JSON(http.StatusOK, limitsJSON(sessionMgr.Limits()))
			return
		}
	}

	sess, err := sessionMgr.Create(c.ClientIP())
	if err != nil {
		switch err {
		case session.ErrDemoFull:
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "This demo is at capacity right now. Please try again in a minute or two.",
				"code":  "demo_full",
			})
		case session.ErrIPLimit:
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Only one active session per visitor is allowed.",
				"code":  "ip_limit",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		}
		return
	}

	limits := sessionMgr.Limits()
	middleware.SetSessionCookie(c, appConfig, sess.ID, int(limits.MaxLifetime.Seconds()))

	c.JSON(http.StatusOK, limitsJSON(limits))
}

// Heartbeat refreshes a session's inactivity timer.
func Heartbeat(c *gin.Context) {
	id, err := c.Cookie(middleware.SessionCookieName)
	if err != nil || id == "" || !sessionMgr.Touch(id) {
		c.JSON(http.StatusGone, gin.H{"error": "Session expired.", "code": "session_expired"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ReleaseSession ends a session immediately (tab-close beacon), freeing its
// slot without waiting for the inactivity timeout, and triggers cleanup of
// its documents/uploads.
func ReleaseSession(c *gin.Context) {
	id, err := c.Cookie(middleware.SessionCookieName)
	if err == nil && id != "" {
		sessionMgr.Release(id)
	}
	middleware.ClearSessionCookie(c, appConfig)
	c.Status(http.StatusNoContent)
}

// SessionStatus reports current usage against limits for the frontend's
// quota UI.
func SessionStatus(c *gin.Context) {
	sess := c.MustGet(middleware.SessionContextKey).(*session.Session)
	snap, ok := sessionMgr.Snapshot(sess.ID)
	if !ok {
		c.JSON(http.StatusGone, gin.H{"error": "Session expired.", "code": "session_expired"})
		return
	}
	limits := sessionMgr.Limits()
	c.JSON(http.StatusOK, gin.H{
		"documents_used": snap.DocCount,
		"max_documents":  limits.MaxDocs,
		"chunks_used":    snap.ChunkCount,
		"max_chunks":     limits.MaxChunks,
		"questions_used": snap.QuestionCount,
		"max_questions":  limits.MaxQuestions,
	})
}
