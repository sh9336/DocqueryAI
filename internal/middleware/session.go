package middleware

import (
	"net/http"

	"assistant/internal/config"
	"assistant/internal/session"

	"github.com/gin-gonic/gin"
)

const SessionCookieName = "docquery_session"
const SessionContextKey = "session"

// SetSessionCookie writes the lease cookie. Cross-origin deployments
// (Vercel frontend + Render backend, different domains) require
// SameSite=None + Secure for the browser to send it back at all; local http
// dev falls back to Lax/non-secure since None+Secure is rejected over http.
func SetSessionCookie(c *gin.Context, cfg *config.Config, sessionID string, maxAgeSeconds int) {
	secure := cfg.Env != "development"
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	c.SetSameSite(sameSite)
	c.SetCookie(SessionCookieName, sessionID, maxAgeSeconds, "/", "", secure, true)
}

// ClearSessionCookie expires the lease cookie immediately.
func ClearSessionCookie(c *gin.Context, cfg *config.Config) {
	SetSessionCookie(c, cfg, "", -1)
}

// RequireSession rejects requests without a live session lease and, on
// success, stores the *session.Session in the Gin context under
// SessionContextKey and refreshes its inactivity timer.
func RequireSession(mgr *session.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := c.Cookie(SessionCookieName)
		if err != nil || id == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "No active session. Please refresh the page.",
				"code":  "session_required",
			})
			return
		}

		sess, ok := mgr.Get(id)
		if !ok {
			c.AbortWithStatusJSON(http.StatusGone, gin.H{
				"error": "Your session expired due to inactivity.",
				"code":  "session_expired",
			})
			return
		}

		mgr.Touch(id)
		c.Set(SessionContextKey, sess)
		c.Next()
	}
}

// RequireDebugToken guards the debug endpoints behind a shared-secret
// header. Callers must only register this middleware when token != "" —
// main.go skips registering the debug routes entirely otherwise so they're
// absent (not just unauthorized) in a default production deployment.
func RequireDebugToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Debug-Token") != token {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Next()
	}
}
