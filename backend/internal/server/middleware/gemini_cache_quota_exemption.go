package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Read/delete operations on already-created caches must remain usable after a
// key's lifetime quota is exhausted. Only exact registered routes qualify; a
// query string, forged suffix, method override or unknown path cannot exempt a
// billable generation, create or renewal request. All authentication, user/group
// authorization, expiration and IP checks still execute in APIKeyAuthGoogle.
func isGeminiCacheQuotaExemptRead(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	switch c.FullPath() {
	case "/v1beta/cachedContents":
		return c.Request.Method == http.MethodGet
	case "/v1beta/cachedContents/:cacheID":
		return c.Request.Method == http.MethodGet || c.Request.Method == http.MethodDelete
	default:
		return false
	}
}
