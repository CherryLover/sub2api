//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFindGeminiCachedContentReferenceRejectsAmbiguousKeys(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate top-level":       `{"cachedContent":"cachedContents/a","contents":[],"cachedContent":"cachedContents/b"}`,
		"snake top-level":           `{"cached_content":"cachedContents/a"}`,
		"snake beside camel":        `{"cachedContent":"cachedContents/a","cached_content":"cachedContents/b"}`,
		"duplicate nested":          `{"generateContentRequest":{"cachedContent":"cachedContents/a","cachedContent":"cachedContents/b"}}`,
		"snake nested":              `{"generateContentRequest":{"cached_content":"cachedContents/a"}}`,
		"snake parent with cache":   `{"generate_content_request":{"cachedContent":"cachedContents/a"}}`,
		"duplicate nested parent":   `{"generateContentRequest":{},"generateContentRequest":{"cachedContent":"cachedContents/a"}}`,
		"conflicting two locations": `{"cachedContent":"cachedContents/a","generateContentRequest":{"cachedContent":"cachedContents/b"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := FindGeminiCachedContentReference([]byte(body))
			require.Error(t, err)
		})
	}

	for name, body := range map[string]string{
		"snake parent without cache": `{"generate_content_request":{"contents":[]}}`,
		"key name inside content":    `{"contents":[{"role":"user","cached_content":"x","parts":[{"text":"cached_content"}]}]}`,
		"not an object":              `[{"cachedContent":"cachedContents/a"}]`,
	} {
		t.Run("allows "+name, func(t *testing.T) {
			ref, err := FindGeminiCachedContentReference([]byte(body))
			require.NoError(t, err)
			require.Nil(t, ref)
		})
	}

	ref, err := FindGeminiCachedContentReference([]byte(`{"cachedContent":"cachedContents/a","generateContentRequest":{"cachedContent":"cachedContents/a"}}`))
	require.NoError(t, err)
	require.Equal(t, []string{"cachedContent", "generateContentRequest.cachedContent"}, ref.Paths)
}

func TestGeminiCachedContentPayloadNormalization(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	maxTTL := 24 * time.Hour

	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","display_name":"docs","expire_time":"2026-10-09T10:00:00Z"}`), now, maxTTL)
	require.NoError(t, err)
	require.Equal(t, "docs", req.DisplayName)
	require.Equal(t, 2*time.Hour, req.TTL)
	for _, key := range []string{"display_name", "expire_time", "displayName", "expireTime"} {
		require.NotContains(t, req.payload, key, "元数据字段不随内容发往上游")
	}

	for name, body := range map[string]string{
		"both spellings": `{"model":"gemini-3.8-flash","displayName":"a","display_name":"b"}`,
		"trailing data":  `{"model":"gemini-3.8-flash"} {"model":"x"}`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := ParseGeminiCachedContentCreateRequest([]byte(body), now, maxTTL)
			require.Error(t, err)
		})
	}

	ttl, err := ParseGeminiCachedContentPatchRequest([]byte(`{"expire_time":"2026-10-09T09:00:00Z"}`), "", now, maxTTL)
	require.NoError(t, err)
	require.Equal(t, time.Hour, ttl)
	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{"display_name":"x"}`), "", now, maxTTL)
	require.ErrorContains(t, err, "immutable")
}

func TestGeminiCachedContentTTLRoundsUpToWholeSeconds(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","ttl":"1e-10s"}`), now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, time.Second, req.TTL, "极小的显式有效期按 1 秒处理，不回落到默认 1 小时")
	require.True(t, req.ttlExplicit)

	ttl, err := ParseGeminiCachedContentPatchRequest([]byte(`{"ttl":"1e-10s"}`), "", now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, time.Second, ttl)

	req, err = ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","expireTime":"2026-10-09T08:00:10.2Z"}`), now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 11*time.Second, req.TTL, "expireTime 折算的有效期与发往上游的整秒 ttl 一致")
}

func TestGeminiCachedContentCreateKeepsNumbersVerbatim(t *testing.T) {
	svc, httpStub := newGeminiCachedContentServiceForTest(t, http.StatusOK,
		`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":2000}}`)
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"models/gemini-3.8-flash","contents":[{"role":"user","parts":[
		{"functionResponse":{"name":"lookup","response":{"order_id":1790000000000000001,"ratio":0.10}}}]}]}`), time.Now(), 24*time.Hour)
	require.NoError(t, err)
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	_, err = svc.CreateUpstream(context.Background(), account, req, "gemini-3.8-flash", "pub1", time.Hour)
	require.NoError(t, err)
	raw, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"order_id":1790000000000000001`)
	require.Contains(t, string(raw), `"ratio":0.10`)
}

func TestGeminiCachedContentCreateUpstreamUnusableResponse(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	for name, tc := range map[string]struct {
		body     string
		wantName string
	}{
		"missing token count": {`{"name":"cachedContents/up9","expireTime":"2099-01-01T00:00:00Z"}`, "cachedContents/up9"},
		"zero token count":    {`{"name":"cachedContents/up9","usageMetadata":{"totalTokenCount":0}}`, "cachedContents/up9"},
		"unexpected name":     {`{"name":"cachedContents/a/../b","usageMetadata":{"totalTokenCount":5}}`, ""},
		"not json":            {`<html>`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := newGeminiCachedContentServiceForTest(t, http.StatusOK, tc.body)
			req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash"}`), time.Now(), 24*time.Hour)
			require.NoError(t, err)
			_, err = svc.CreateUpstream(context.Background(), account, req, "gemini-3.8-flash", "pub1", time.Hour)
			var malformed *GeminiCachedContentMalformedResponseError
			require.ErrorAs(t, err, &malformed)
			require.Equal(t, tc.wantName, malformed.Name)
		})
	}
}

type geminiCachedContentCtxProbeUpstream struct {
	geminiCompatHTTPUpstreamStub
	ctxErrAtSend error
}

func (s *geminiCachedContentCtxProbeUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	s.ctxErrAtSend = req.Context().Err()
	return s.geminiCompatHTTPUpstreamStub.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGeminiCachedContentUpstreamCallIgnoresClientCancel(t *testing.T) {
	up := &geminiCachedContentCtxProbeUpstream{geminiCompatHTTPUpstreamStub: geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":2000}}`)),
	}}}
	svc := NewGeminiCachedContentService(newGeminiCachedContentRepoStub(),
		&GeminiMessagesCompatService{httpUpstream: up, cfg: &config.Config{}}, &config.Config{})
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash"}`), time.Now(), 24*time.Hour)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	_, err = svc.CreateUpstream(ctx, account, req, "gemini-3.8-flash", "pub1", time.Hour)
	require.NoError(t, err)
	require.NoError(t, up.ctxErrAtSend, "客户端断开不取消发往上游的创建")
}

func TestClampGeminiCachedContentExpire(t *testing.T) {
	limit := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	require.Equal(t, limit, ClampGeminiCachedContentExpire(time.Time{}, limit))
	require.Equal(t, limit, ClampGeminiCachedContentExpire(limit.Add(100*time.Hour), limit), "上游回报的到期时间不超过请求的有效期")
	earlier := limit.Add(-time.Second)
	require.Equal(t, earlier, ClampGeminiCachedContentExpire(earlier, limit))
}
