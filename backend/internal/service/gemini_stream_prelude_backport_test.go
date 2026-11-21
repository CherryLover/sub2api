package service

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiUnspecifiedBlockReasonIsNotCompletion(t *testing.T) {
	for _, reason := range []string{"BLOCK_REASON_UNSPECIFIED", "BLOCKED_REASON_UNSPECIFIED", ""} {
		require.Empty(t, geminiStreamTerminalReason([]byte(`{"promptFeedback":{"blockReason":"`+reason+`"}}`)))
	}
	require.Equal(t, "SAFETY", geminiStreamTerminalReason([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`)))
}
func TestGeminiStreamEmptyPreludeRemainsRetryable(t *testing.T) {
	for _, protocol := range []string{"native", "messages", "chat"} {
		for _, body := range []string{"", "data: {\"usageMetadata\":{\"promptTokenCount\":7}}\n\n", "data: {\"promptFeedback\":{\"blockReason\":\"BLOCK_REASON_UNSPECIFIED\"}}\n\n"} {
			t.Run(protocol+"/"+body, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil)
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				svc := &GeminiMessagesCompatService{}
				var err error
				switch protocol {
				case "native":
					_, err = svc.handleNativeStreamingResponse(c, resp, time.Now(), false)
				case "messages":
					_, err = svc.handleStreamingResponse(c, resp, time.Now(), "gemini-test")
				case "chat":
					_, err = svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-test", false, true)
				}
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.True(t, failure.EmptyStream)
				require.False(t, c.Writer.Written(), "prelude alone must not commit a success or error payload")
			})
		}
	}
}
func TestGeminiStreamCanceledBeforeOutputNeverReplays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil).WithContext(ctx)
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
	_, err := (&GeminiMessagesCompatService{}).handleNativeStreamingResponse(c, resp, time.Now(), false)
	require.ErrorIs(t, err, context.Canceled)
	var failure *UpstreamFailoverError
	require.False(t, errors.As(err, &failure))
	require.False(t, c.Writer.Written())
}
