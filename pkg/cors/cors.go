package cors

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// IsOriginAllowed checks if the given origin is part of the allowed list.
// If the allowed list is empty, it returns false (nothing is allowed).
func IsOriginAllowed(origin string, allowed []string) bool {
	if origin == "" {
		// No Origin header: usually not a browser CORS request.
		// We treat this as "no CORS processing required" → caller decides.
		return false
	}
	for _, a := range allowed {
		if origin == a {
			return true
		}
	}
	return false
}

// ApplyCORS applies CORS headers based on the given list of allowed origins.
// It returns false if:
//   - this is a CORS request and the origin is NOT allowed (403 already sent), or
//   - this is a preflight (OPTIONS) request (204 already sent).
//
// If it returns true, the handler may continue processing the request.
func ApplyCORS(c *gin.Context, allowedOrigins []string) bool {
	origin := c.GetHeader("Origin")

	// If there is no Origin header, it's not a browser CORS request.
	// In that case, we do not apply any CORS logic here.
	if origin == "" {
		return true
	}

	if len(allowedOrigins) == 0 || !IsOriginAllowed(origin, allowedOrigins) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "origin_not_allowed",
		})
		return false
	}

	// Dynamically allow the requesting origin
	c.Header("Access-Control-Allow-Origin", origin)
	c.Header("Vary", "Origin")
	c.Header("Access-Control-Allow-Credentials", "true")

	// Allow typical headers and methods used by your frontend
	c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With, Accept, Origin")

	// Handle preflight
	if c.Request.Method == http.MethodOptions {
		c.Status(http.StatusNoContent)
		return false
	}

	return true
}
