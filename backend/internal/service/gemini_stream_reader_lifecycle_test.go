package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type geminiLifecycleBody struct {
	io.ReadCloser
	closed atomic.Int32
}

func (b *geminiLifecycleBody) Close() error { b.closed.Add(1); return b.ReadCloser.Close() }
func TestGeminiPreludeIdleHeartbeatDoesNotExtendDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil)
		pr, pw := io.Pipe()
		defer func() { _ = pw.Close() }()
		body := &geminiLifecycleBody{ReadCloser: pr}
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}
		cfg := &config.Config{}
		cfg.Gateway.StreamKeepaliveInterval = 1
		cfg.Gateway.StreamDataIntervalTimeout = 2
		start := time.Now()
		_, _, err := (&GeminiMessagesCompatService{cfg: cfg}).prepareGeminiStream(c, resp, "native")
		require.Equal(t, 2*time.Second, time.Since(start))
		var failure *UpstreamFailoverError
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.SafeToFailoverAfterWrite)
		require.True(t, failure.EmptyStream)
		require.Contains(t, rec.Body.String(), ": ping")
		require.NotContains(t, rec.Body.String(), "message_start")
		require.NoError(t, resp.Body.Close())
		require.EqualValues(t, 1, body.closed.Load())
	})
}
func TestGeminiPreludeRetainsUsageAndFirstContentExactlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil)
	body := "data: {\"usageMetadata\":{\"promptTokenCount\":7}}\n\n" + "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"once\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":7,\"candidatesTokenCount\":2}}\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	result, err := (&GeminiMessagesCompatService{}).handleStreamingResponse(c, resp, time.Now(), "public-model")
	require.NoError(t, err)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 2, result.usage.OutputTokens)
	require.Equal(t, 1, strings.Count(rec.Body.String(), `"text":"once"`))
	require.Contains(t, rec.Body.String(), "message_stop")
}
func TestGeminiPreludeCanceledReaderClosesUpstreamOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil).WithContext(ctx)
		pr, pw := io.Pipe()
		defer func() { _ = pw.Close() }()
		body := &geminiLifecycleBody{ReadCloser: pr}
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}
		done := make(chan error, 1)
		go func() {
			_, _, err := (&GeminiMessagesCompatService{}).prepareGeminiStream(c, resp, "chat")
			done <- err
		}()
		synctest.Wait()
		cancel()
		err := <-done
		require.ErrorIs(t, err, context.Canceled)
		require.False(t, c.Writer.Written())
		var failure *UpstreamFailoverError
		require.False(t, errors.As(err, &failure))
		require.NoError(t, resp.Body.Close())
		require.EqualValues(t, 1, body.closed.Load())
	})
}
func TestGeminiPreludePermissionErrorNeverReplaysIdentity(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/fixture", nil)
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"error\":{\"code\":403,\"message\":\"private project-id\"}}\n\n"))}
	_, _, err := (&GeminiMessagesCompatService{}).prepareGeminiStream(c, resp, "native")
	require.Error(t, err)
	var failure *UpstreamFailoverError
	require.False(t, errors.As(err, &failure))
	require.Equal(t, 403, c.Writer.Status())
}
