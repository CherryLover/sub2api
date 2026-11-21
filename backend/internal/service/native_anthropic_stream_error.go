package service

import "github.com/gin-gonic/gin"

// A transport read error must not be finalized as a successful Chat/Responses
// generation. Keep the error in SSE and preserve accumulated usage for the caller.
func writeNativeAnthropicCompatStreamReadError(c *gin.Context) {
	writeSSE(c.Writer, "error", map[string]any{
		"type": "error",
		"error": map[string]any{
			"type": "server_error", "code": "upstream_read_error",
			"message": "Upstream stream read failed",
		},
	})
	c.Writer.Flush()
	MarkResponseCommitted(c)
}
