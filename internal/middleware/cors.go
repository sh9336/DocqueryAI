package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS returns a Gin middleware that adds the correct Access-Control-* headers
// for every request and handles OPTIONS preflight requests.
//
// allowedOrigins is a comma-separated list of exact origins that are permitted
// (e.g. "http://localhost:3000,https://app.example.com").
// A wildcard "*" is intentionally NOT supported to avoid accidentally allowing
// untrusted origins in production.
func CORS(allowedOrigins string) gin.HandlerFunc {
	// Parse once at startup — O(1) lookup per request
	originSet := make(map[string]struct{})
	for _, o := range strings.Split(allowedOrigins, ",") {
		origin := strings.TrimSpace(o)
		if origin != "" {
			originSet[origin] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if _, allowed := originSet[origin]; allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin") // Required when not using wildcard
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Accept")
			c.Header("Access-Control-Max-Age", "86400") // Cache preflight for 24 h
		}

		// Handle preflight — must return 204 before any handler runs
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
