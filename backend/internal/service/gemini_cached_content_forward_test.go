package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentNativeBoundErrorsDoNotRetry(t *testing.T) {
	for _, action := range []string{"generateContent", "countTokens"} {
		for _, status := range []int{429, 503} {
			t.Run(fmt.Sprintf("%s/%d", action, status), func(t *testing.T) {
				up := &geminiCompatHTTPUpstreamStub{response: &http.Response{
					StatusCode: status, Header: http.Header{},
					Body: io.NopCloser(strings.NewReader(`{"error":{"message":"private upstream details"}}`)),
				}}
				svc := &GeminiMessagesCompatService{httpUpstream: up, cfg: &config.Config{}}
				account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
					"api_key": "test", "base_url": "https://cache.example.test", "explicit_cache_upstream": true,
				}}
				body := `{"cachedContent":"cachedContents/up1","contents":[{"role":"user","parts":[{"text":"question"}]}]}`
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:"+action, strings.NewReader(body))
				result, err := svc.ForwardNative(WithGeminiCachedContentBound(context.Background()), c, account, "gemini-2.5-pro", action, false, []byte(body))
				require.Error(t, err)
				require.Nil(t, result)
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.Equal(t, 1, up.calls)
				require.Equal(t, status, rec.Code)
				require.NotContains(t, rec.Body.String(), "private upstream")
				require.NotContains(t, rec.Body.String(), "totalTokens")
			})
		}
	}
}

func TestGeminiCachedContentNativeNotFoundNoAccountPenalty(t *testing.T) {
	for _, tc := range []struct {
		status  int
		message string
	}{
		{403, "CachedContent not found (or permission denied)"},
		{400, "Invalid resource state for cache content 123."},
		{404, "Cached content 123 is not found."},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			up := &geminiCompatHTTPUpstreamStub{response: &http.Response{
				StatusCode: tc.status, Header: http.Header{},
				Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":{"message":%q}}`, tc.message))),
			}}
			svc := &GeminiMessagesCompatService{httpUpstream: up, cfg: &config.Config{}}
			account := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
			body := `{"cachedContent":"cachedContents/up1"}`
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:countTokens", strings.NewReader(body))
			_, err := svc.ForwardNative(WithGeminiCachedContentBound(context.Background()), c, account, "gemini-2.5-pro", "countTokens", false, []byte(body))
			require.Error(t, err)
			require.Equal(t, 1, up.calls)
			require.Equal(t, http.StatusForbidden, rec.Code)
			require.JSONEq(t, GeminiCachedContentNotFoundResponse, rec.Body.String())
		})
	}
}

func TestGeminiCachedContentNativeUnrelatedAuthErrorPreserved(t *testing.T) {
	svc := &GeminiMessagesCompatService{}
	body := `{"error":{"message":"Invalid API key"}}`
	resp := &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(body))}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	handled, err := svc.handleGeminiCachedContentForwardError(context.Background(), c, &Account{}, resp, []byte(`{"cachedContent":"cachedContents/up1"}`))
	require.False(t, handled)
	require.NoError(t, err)
	remaining, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(remaining))
}
