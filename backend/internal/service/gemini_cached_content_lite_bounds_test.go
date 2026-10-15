//go:build unit

package service

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentLiteDurationBounds(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	_, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-2.5-pro","expireTime":"9999-01-01T00:00:00Z"}`), now, 24*time.Hour)
	require.Error(t, err)
	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{"expireTime":"9999-01-01T00:00:00Z"}`), "", now, 24*time.Hour)
	require.Error(t, err)
	for _, value := range []any{math.Inf(1), math.NaN(), 1e30, "1e30", -1} {
		account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{
			GeminiExplicitCacheUpstreamCredentialKey:       true,
			GeminiExplicitCacheUpstreamMaxTTLCredentialKey: value,
		}}
		require.Zero(t, account.GeminiExplicitCacheUpstreamMaxTTL())
	}
	cfg := &config.Config{}
	cfg.Gateway.GeminiCachedContentMaxTTLSeconds = math.MaxInt
	require.Equal(t, 24*time.Hour, NewGeminiCachedContentService(nil, nil, cfg).MaxTTL())
}

func TestGeminiCachedContentLiteResponseValidation(t *testing.T) {
	_, err := parseGeminiCachedContentUpstream([]byte(`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":10}},garbage`))
	require.Error(t, err)
	body := `{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":10},"padding":"` + strings.Repeat("x", geminiCachedContentResponseMaxBytes) + `"}`
	svc, _ := newGeminiCachedContentServiceForTest(t, http.StatusOK, body)
	account := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
	request, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-2.5-pro"}`), time.Now(), time.Hour)
	require.NoError(t, err)
	_, err = svc.CreateUpstream(context.Background(), account, request, "gemini-2.5-pro", "public1", time.Hour)
	var malformed *GeminiCachedContentMalformedResponseError
	require.ErrorAs(t, err, &malformed)
	require.Contains(t, err.Error(), "size limit")
}

func TestGeminiCachedContentLiteBoundGatewayErrorClassification(t *testing.T) {
	gateway := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		GeminiExplicitCacheUpstreamCredentialKey: true,
	}}
	bound := WithGeminiCachedContentBound(context.Background())
	require.True(t, isGeminiBoundCacheUpstreamPassthrough(bound, gateway, http.StatusTooManyRequests))
	require.True(t, isGeminiBoundCacheUpstreamPassthrough(bound, gateway, http.StatusServiceUnavailable))
	require.False(t, isGeminiBoundCacheUpstreamPassthrough(bound, gateway, http.StatusForbidden))
	require.False(t, isGeminiBoundCacheUpstreamPassthrough(context.Background(), gateway, http.StatusTooManyRequests))
	require.False(t, isGeminiBoundCacheUpstreamPassthrough(bound, &Account{Type: AccountTypeAPIKey}, http.StatusTooManyRequests))
	require.False(t, geminiCachedContentBoundFromContext(nil))
	require.False(t, (&GeminiCachedContentUpstreamError{StatusCode: 403, Body: []byte(`{"error":{"message":"Invalid API key"}}`)}).NotFound())
}
