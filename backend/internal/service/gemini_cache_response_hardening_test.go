//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentRenewalRequiresConfirmedExpiry(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}}
	record := &GeminiCachedContent{UpstreamName: "cachedContents/up1"}
	for _, body := range []string{
		`<html>temporary error</html>`, `{}`, `[]`,
		`{"name":"cachedContents/up1","expireTime":"invalid"}`,
		`{"name":"cachedContents/another","expireTime":"2099-01-01T00:00:00Z"}`,
	} {
		t.Run(body, func(t *testing.T) {
			svc, _ := newGeminiCachedContentServiceForTest(t, http.StatusOK, body)
			_, err := svc.PatchUpstream(context.Background(), account, record, time.Hour)
			var malformed *GeminiCachedContentMalformedResponseError
			require.ErrorAs(t, err, &malformed)
			require.Equal(t, record.UpstreamName, malformed.Name)
		})
	}
	svc, up := newGeminiCachedContentServiceForTest(t, http.StatusOK, `{"name":"cachedContents/up1","expireTime":"2099-01-01T00:00:00Z"}`)
	result, err := svc.PatchUpstream(context.Background(), account, record, time.Hour)
	require.NoError(t, err)
	require.Equal(t, 2099, result.ExpireTime.Year())
	require.Equal(t, http.MethodPatch, up.lastReq.Method)
	require.Equal(t, "ttl", up.lastReq.URL.Query().Get("updateMask"))
}

func TestGeminiCachedContentCreateRejectsAmbiguousUsage(t *testing.T) {
	for _, body := range []string{
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":"1000"}}`,
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":-1}}`,
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":1.5}}`,
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":1e1000000}}`,
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":1000},"expireTime":null}`,
	} {
		_, err := parseGeminiCachedContentUpstream([]byte(body))
		var malformed *GeminiCachedContentMalformedResponseError
		require.ErrorAs(t, err, &malformed)
		require.Equal(t, "cachedContents/up1", malformed.Name, "known resources remain available for cleanup")
	}
}

func TestGeminiCacheStoragePriceUsesFixedPricingTime(t *testing.T) {
	billing := &BillingService{}
	before := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	after := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	require.InDelta(t, 0.5e-6, billing.cacheStoragePriceAt("gemini-3.8-flash", before), 1e-15)
	require.InDelta(t, 1e-6, billing.cacheStoragePriceAt("gemini-3.8-flash", after), 1e-15)
	for _, model := range []string{"gemini-3.8-flash-lite", "gemini-3.8-flash-cyber", "gemini-9-flash"} {
		require.InDelta(t, 1e-6, billing.cacheStoragePriceAt(model, before), 1e-15)
	}
	require.Zero(t, billing.cacheStoragePriceAt("unknown-model", before))
}
