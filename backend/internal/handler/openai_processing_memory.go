package handler

import (
	"errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

func (h *OpenAIGatewayHandler) admitOpenAIJSONProcessing(c *gin.Context, body []byte, anthropic bool) bool {
	err := pkghttputil.ReserveJSONProcessingMemory(c.Request.Context(), body)
	if err == nil {
		return true
	}
	h.handleOpenAIProcessingMemoryError(c, err, anthropic, false)
	return false
}
func (h *OpenAIGatewayHandler) handleOpenAIProcessingMemoryError(c *gin.Context, err error, anthropic, streamStarted bool) bool {
	status, kind, message := http.StatusServiceUnavailable, "api_error", "Request body memory budget exhausted"
	switch {
	case errors.Is(err, pkghttputil.ErrRequestBodyMemoryTooLarge):
		status, kind, message = http.StatusRequestEntityTooLarge, "invalid_request_error", "Request body processing exceeds configured memory budget"
		c.Writer.Header().Del("Retry-After")
	case errors.Is(err, pkghttputil.ErrRequestBodyMemoryBudget):
		c.Header("Retry-After", "1")
	default:
		return false
	}
	streamStarted = streamStarted || (c.Writer.Written() && strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream"))
	if anthropic {
		h.anthropicStreamingAwareError(c, status, kind, message, streamStarted)
	} else {
		h.handleStreamingAwareError(c, status, kind, message, streamStarted)
	}
	return true
}
