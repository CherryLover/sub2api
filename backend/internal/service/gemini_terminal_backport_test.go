//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiStreamTerminalStateMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	geminiSvc := &GeminiMessagesCompatService{
		cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
	}
	agSvc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)

	const textChunk = `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"partial answer"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2}}` + "\n\n"
	const stopChunk = `data: {"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":4}}` + "\n\n"

	type entrypoint struct {
		name               string
		path               string
		run                func(c *gin.Context, body io.ReadCloser) error
		wantCompleteMarker string
		forbidOnIncomplete []string
		wantErrorMarker    string
	}
	entrypoints := []entrypoint{
		{
			name: "gemini_v1beta",
			path: "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleNativeStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), false)
				return err
			},
			wantCompleteMarker: `"finishReason":"STOP"`,
			wantErrorMarker:    `"error":{`,
		},
		{
			name: "antigravity_v1beta",
			path: "/v1beta/models/gemini-3-pro:streamGenerateContent",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleGeminiStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now())
				return err
			},
			wantCompleteMarker: `"finishReason":"STOP"`,
			wantErrorMarker:    `"error":{`,
		},
		{
			name: "gemini_messages",
			path: "/v1/messages",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-2.5-pro")
				return err
			},
			wantCompleteMarker: "event: message_stop",
			forbidOnIncomplete: []string{`"stop_reason":"end_turn"`, "event: message_stop"},
			wantErrorMarker:    "event: error",
		},
		{
			name: "antigravity_messages",
			path: "/v1/messages",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleClaudeStreamingResponse(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro")
				return err
			},
			wantCompleteMarker: "event: message_stop",
			forbidOnIncomplete: []string{`"stop_reason":"end_turn"`, "event: message_stop"},
			wantErrorMarker:    "event: error",
		},
		{
			name: "gemini_chat_completions",
			path: "/v1/chat/completions",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := geminiSvc.handleChatCompletionsStreamingResponseFromGemini(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-2.5-pro", false, false)
				return err
			},
			wantCompleteMarker: "data: [DONE]",
			forbidOnIncomplete: []string{`"finish_reason":"stop"`, "data: [DONE]"},
			wantErrorMarker:    "event: error",
		},
		{
			name: "antigravity_chat_completions",
			path: "/v1/chat/completions",
			run: func(c *gin.Context, body io.ReadCloser) error {
				_, err := agSvc.handleChatCompletionsStreamingFromAntigravity(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, time.Now(), "gemini-3-pro", false)
				return err
			},
			wantCompleteMarker: "data: [DONE]",
			forbidOnIncomplete: []string{`"finish_reason":"stop"`, "data: [DONE]"},
			wantErrorMarker:    `"error":{`,
		},
	}

	for _, ep := range entrypoints {
		t.Run(ep.name+"/stop_completes", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, io.NopCloser(strings.NewReader(textChunk+stopChunk)))
			require.NoError(t, err)
			out := rec.Body.String()
			require.Contains(t, out, "partial answer")
			require.Contains(t, out, ep.wantCompleteMarker)
		})

		t.Run(ep.name+"/clean_eof_without_finish_reason_errors", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, io.NopCloser(strings.NewReader(textChunk)))
			require.Error(t, err)
			out := rec.Body.String()
			require.Contains(t, out, "partial answer")
			require.Contains(t, out, ep.wantErrorMarker)
			for _, forbidden := range ep.forbidOnIncomplete {
				require.NotContains(t, out, forbidden)
			}
		})

		t.Run(ep.name+"/unexpected_eof_without_finish_reason_errors", func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, ep.path, nil)
			err := ep.run(c, &antigravityCompatErrorReader{data: []byte(textChunk), err: io.ErrUnexpectedEOF})
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			out := rec.Body.String()
			require.Contains(t, out, "partial answer")
			for _, forbidden := range ep.forbidOnIncomplete {
				require.NotContains(t, out, forbidden)
			}
		})
	}
}
