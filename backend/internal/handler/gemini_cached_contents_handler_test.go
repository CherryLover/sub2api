//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiCacheHandlerRepo struct {
	service.GeminiCachedContentRepository
	records []*service.GeminiCachedContent
	reads   int
}

func (r *geminiCacheHandlerRepo) GetForOwner(_ context.Context, keyID int64, id string) (*service.GeminiCachedContent, error) {
	r.reads++
	for _, v := range r.records {
		if v.APIKeyID == keyID && v.PublicID == id {
			copy := *v
			return &copy, nil
		}
	}
	return nil, service.ErrGeminiCachedContentNotFound
}
func (r *geminiCacheHandlerRepo) ListForOwner(_ context.Context, keyID, groupID int64, activeAt time.Time, before int64, limit int) ([]*service.GeminiCachedContent, error) {
	r.reads++
	out := []*service.GeminiCachedContent{}
	for _, v := range r.records {
		if v.APIKeyID == keyID && v.GroupID == groupID && v.ExpireTime.After(activeAt) && (before <= 0 || v.ID < before) {
			out = append(out, v)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}
func newGeminiCacheHandlerTest(t *testing.T, method, path, body string) (*GatewayHandler, *gin.Context, *httptest.ResponseRecorder, *geminiCacheHandlerRepo, *service.APIKey) {
	t.Helper()
	key := &service.APIKey{ID: 11, UserID: 12, User: &service.User{ID: 12}, Group: &service.Group{ID: 13, Platform: service.PlatformGemini}}
	key.GroupID = &key.Group.ID
	now := time.Now().UTC()
	repo := &geminiCacheHandlerRepo{records: []*service.GeminiCachedContent{{ID: 1, PublicID: "public1", APIKeyID: key.ID, UserID: key.UserID, GroupID: *key.GroupID, AccountID: 14, UpstreamName: "cachedContents/privateproviderid", UpstreamModel: "models/gemini-2.5-pro", Model: "gemini-2.5-pro", ExpireTime: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now, TotalTokenCount: 1000}}}
	h := &GatewayHandler{geminiCachedContentService: service.NewGeminiCachedContentService(repo, nil, &config.Config{})}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Set(string(middleware.ContextKeyAPIKey), key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.UserID, Concurrency: 2})
	c.Params = gin.Params{{Key: "cacheID", Value: "public1"}}
	return h, c, rec, repo, key
}
func TestGeminiCachedContentsGetOwnerIsolation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*geminiCacheHandlerRepo, *service.APIKey)
	}{
		{"other key", func(r *geminiCacheHandlerRepo, k *service.APIKey) { k.ID++ }},
		{"other group", func(r *geminiCacheHandlerRepo, k *service.APIKey) { r.records[0].GroupID++ }},
		{"other user", func(r *geminiCacheHandlerRepo, k *service.APIKey) { r.records[0].UserID++ }},
		{"expired", func(r *geminiCacheHandlerRepo, k *service.APIKey) {
			r.records[0].ExpireTime = time.Now().Add(-time.Second)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, c, w, r, k := newGeminiCacheHandlerTest(t, http.MethodGet, "/v1beta/cachedContents/public1", "")
			tc.mutate(r, k)
			h.GeminiCachedContentsGet(c)
			require.Equal(t, 403, w.Code)
			require.JSONEq(t, service.GeminiCachedContentNotFoundResponse, w.Body.String())
			require.NotContains(t, w.Body.String(), "privateproviderid")
		})
	}
	h, c, w, _, _ := newGeminiCacheHandlerTest(t, http.MethodGet, "/v1beta/cachedContents/public1", "")
	h.GeminiCachedContentsGet(c)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "cachedContents/public1")
	require.NotContains(t, w.Body.String(), "privateproviderid")
	require.NotContains(t, w.Body.String(), "account_id")
}
func TestGeminiCachedContentsListPaginationAndMalformedCursor(t *testing.T) {
	h, c, w, r, k := newGeminiCacheHandlerTest(t, http.MethodGet, "/v1beta/cachedContents?pageSize=1", "")
	second := *r.records[0]
	second.ID = 2
	second.PublicID = "public2"
	r.records = append([]*service.GeminiCachedContent{&second}, r.records...)
	other := second
	other.ID = 3
	other.APIKeyID = k.ID + 10
	r.records = append([]*service.GeminiCachedContent{&other}, r.records...)
	h.GeminiCachedContentsList(c)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "public2")
	require.NotContains(t, w.Body.String(), "public1")
	require.Contains(t, w.Body.String(), "nextPageToken")
	for _, query := range []string{"pageSize=-1", "pageToken=!", "pageToken=" + strings.Repeat("a", 1000)} {
		h, c, w, r, _ := newGeminiCacheHandlerTest(t, http.MethodGet, "/v1beta/cachedContents?"+query, "")
		h.GeminiCachedContentsList(c)
		require.Equal(t, 400, w.Code)
		require.Zero(t, r.reads)
	}
}
func TestGeminiCachedContentBindingValidationAndRewrite(t *testing.T) {
	h, c, w, _, k := newGeminiCacheHandlerTest(t, http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", "")
	bound, body, ok := h.resolveGeminiCachedContentBinding(c, k, "gemini-2.5-pro", []byte(`{"cachedContent":"cachedContents/public1","contents":[]}`))
	require.True(t, ok, w.Body.String())
	require.EqualValues(t, 14, bound.AccountID)
	require.Contains(t, string(body), "cachedContents/privateproviderid")
	for _, tc := range []struct{ name, model, body string }{
		{"different model", "gemini-2.5-flash", `{"cachedContent":"cachedContents/public1"}`},
		{"raw upstream", "gemini-2.5-pro", `{"cachedContent":"cachedContents/privateproviderid"}`},
		{"duplicate reference", "gemini-2.5-pro", `{"cachedContent":"cachedContents/public1","cachedContent":"cachedContents/public2"}`},
		{"snake alias", "gemini-2.5-pro", `{"cached_content":"cachedContents/public1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, c, _, _, k := newGeminiCacheHandlerTest(t, http.MethodPost, "/v1beta/models/x:countTokens", "")
			_, _, ok := h.resolveGeminiCachedContentBinding(c, k, tc.model, []byte(tc.body))
			require.False(t, ok)
		})
	}
}
func TestGeminiCachedContentsCreateRejectsInvalidInputBeforeUpstream(t *testing.T) {
	for _, body := range []string{`{}`, `{"model":"gemini/../../escape"}`, `{"model":"gemini-2.5-pro","ttl":"-1s"}`, `{"model":"gemini-2.5-pro","ttl":"9999999999999999s"}`, `{"model":"gemini-2.5-pro","ttl":"1s","expireTime":"2099-01-01T00:00:00Z"}`} {
		h, c, w, _, _ := newGeminiCacheHandlerTest(t, http.MethodPost, "/v1beta/cachedContents", body)
		h.GeminiCachedContentsCreate(c)
		require.Equal(t, 400, w.Code, body)
	}
}
func TestGeminiCachedContentsEndpointNormalization(t *testing.T) {
	require.Equal(t, EndpointGeminiCachedContents, NormalizeInboundEndpoint("/v1beta/cachedContents/secret-resource-id"))
	require.NotEqual(t, EndpointGeminiCachedContents, NormalizeInboundEndpoint("/v1beta/cachedContents-evil"))
}
