package service

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errGeminiStreamIncomplete = errors.New("stream incomplete: missing finishReason")

// A clean transport EOF does not establish that generation completed. Gemini
// reports a candidate finish reason, or prompt feedback when generation is blocked.
func geminiStreamTerminalReason(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	root := gjson.ParseBytes(body)
	if response := root.Get("response"); response.IsObject() {
		root = response
	}
	reason := strings.ToUpper(strings.TrimSpace(root.Get("candidates.0.finishReason").String()))
	if reason != "" && reason != "FINISH_REASON_UNSPECIFIED" {
		return reason
	}
	block := strings.ToUpper(strings.TrimSpace(root.Get("promptFeedback.blockReason").String()))
	if block != "" && block != "BLOCK_REASON_UNSPECIFIED" && block != "BLOCKED_REASON_UNSPECIFIED" {
		return "SAFETY"
	}
	return ""
}

// Keep errors inside the active SSE protocol; never append a JSON HTTP response
// or a successful completion marker to a partially delivered generation.
func writeGeminiIncompleteStreamError(c *gin.Context, protocol string) {
	const message = "Upstream stream ended without a completion signal"
	event := ""
	var payload any
	switch protocol {
	case "native":
		payload = map[string]any{"error": map[string]any{
			"code": http.StatusBadGateway, "status": "UNAVAILABLE", "message": message,
		}}
	case "messages":
		event = "error"
		payload = map[string]any{"type": "error", "error": map[string]any{
			"type": "api_error", "message": message,
		}}
	default:
		event = "error"
		payload = map[string]any{"error": map[string]any{
			"type": "server_error", "code": "stream_incomplete", "message": message,
		}}
	}
	writeSSE(c.Writer, event, payload)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	MarkResponseCommitted(c)
}
