package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Only an explicit no-semantic-output proof allows failover after bytes were sent.
func gatewayForwardMayFailover(c *gin.Context, before int, err *service.UpstreamFailoverError) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	return c.Writer.Size() == before || (err != nil && err.SafeToFailoverAfterWrite)
}

// A committed Gemini stream must end with a Google-shaped data event, not raw
// JSON appended after an SSE heartbeat. Uncommitted errors keep HTTP semantics.
func writeGeminiFailoverError(c *gin.Context, status int, message string) {
	defer middleware.MarkAbortTruncatedStream(c)
	if c.Writer.Written() {
		if service.IsResponseCommitted(c) {
			return
		}
		service.MarkResponseCommitted(c)
		service.MarkOpsStreamError(c, "upstream_error", message, status)
		c.SSEvent("", gin.H{"error": gin.H{"code": status, "message": message, "status": googleapi.HTTPStatusToGoogleStatus(status)}})
		c.Writer.Flush()
		return
	}
	googleError(c, status, message)
}
