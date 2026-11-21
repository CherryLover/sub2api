package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayForwardMayFailoverRequiresProofAfterWrite(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	before := c.Writer.Size()
	require.True(t, gatewayForwardMayFailover(c, before, nil))
	_, err := c.Writer.WriteString(": ping\n\n")
	require.NoError(t, err)
	require.False(t, gatewayForwardMayFailover(c, before, &service.UpstreamFailoverError{}))
	require.True(t, gatewayForwardMayFailover(c, before, &service.UpstreamFailoverError{SafeToFailoverAfterWrite: true}))
}
func TestGatewayFirstByteExhaustedAfterHeartbeatWritesTerminalOnce(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			c.Header("Content-Type", "text/event-stream")
			_, err := c.Writer.WriteString(": ping\n\n")
			require.NoError(t, err)
			c.Writer.Flush()
			h := &GatewayHandler{}
			failure := &service.UpstreamFailoverError{StatusCode: 504, SafeToFailoverAfterWrite: true}
			switch path {
			case "/v1/responses":
				h.handleResponsesFailoverExhausted(c, failure, false)
				h.handleResponsesFailoverExhausted(c, failure, false)
			case "/v1/chat/completions":
				h.handleCCFailoverExhausted(c, failure, false)
				h.handleCCFailoverExhausted(c, failure, false)
			default:
				h.handleFailoverExhausted(c, failure, service.PlatformAntigravity, false)
			}
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "error")
			if path == "/v1/responses" {
				require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
			}
			if path == "/v1/chat/completions" {
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"error"`))
			}
		})
	}
}

func TestGeminiFirstByteExhaustionKeepsSSEFraming(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:streamGenerateContent", nil)
	c.Header("Content-Type", "text/event-stream")
	_, err := c.Writer.WriteString(":\n\n")
	require.NoError(t, err)
	c.Writer.Flush()
	writeGeminiFailoverError(c, http.StatusBadGateway, "retry later")
	writeGeminiFailoverError(c, http.StatusBadGateway, "retry later")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "data:"))
	require.Contains(t, rec.Body.String(), `"code":502`)
	require.Contains(t, rec.Body.String(), `"status":"INTERNAL"`)
}
