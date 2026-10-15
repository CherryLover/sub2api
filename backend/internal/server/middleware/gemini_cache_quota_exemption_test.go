package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiCacheQuotaExemptionKeepsAuthenticationAndWriteLimits(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, status string
		expired                    bool
		want                       int
	}{
		{"list after exhaustion", "GET", "/v1beta/cachedContents", "quota_exhausted", false, 200},
		{"get after exhaustion", "GET", "/v1beta/cachedContents/owned", "quota_exhausted", false, 200},
		{"delete after exhaustion", "DELETE", "/v1beta/cachedContents/owned", "quota_exhausted", false, 200},
		{"create still rejected", "POST", "/v1beta/cachedContents", "quota_exhausted", false, 429},
		{"renew still rejected", "PATCH", "/v1beta/cachedContents/owned", "quota_exhausted", false, 429},
		{"generation not exempt", "POST", "/v1beta/models/gemini:generateContent?cache=/v1beta/cachedContents", "quota_exhausted", false, 429},
		{"forged prefix not exempt", "GET", "/v1beta/cachedContents-evil", "quota_exhausted", false, 429},
		{"disabled cannot read", "GET", "/v1beta/cachedContents", "disabled", false, 401},
		{"expired cannot delete", "DELETE", "/v1beta/cachedContents/owned", "active", true, 403},
		{"runtime exhausted key can read", "GET", "/v1beta/cachedContents", "active", false, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gid := int64(13)
			key := &service.APIKey{ID: 11, Key: "sk-test", UserID: 12, User: &service.User{ID: 12, Status: service.StatusActive, Concurrency: 2}, Status: tc.status, GroupID: &gid, Group: &service.Group{ID: gid, Platform: service.PlatformGemini, Status: service.StatusActive}, Quota: 1, QuotaUsed: 1}
			if tc.expired {
				expired := time.Now().Add(-time.Minute)
				key.ExpiresAt = &expired
			}
			svc := newTestAPIKeyService(fakeAPIKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return key, nil }})
			router := gin.New()
			router.Use(APIKeyAuthGoogle(svc, &config.Config{}))
			ok := func(c *gin.Context) { c.Status(http.StatusOK) }
			router.GET("/v1beta/cachedContents", ok)
			router.POST("/v1beta/cachedContents", ok)
			router.GET("/v1beta/cachedContents/:cacheID", ok)
			router.PATCH("/v1beta/cachedContents/:cacheID", ok)
			router.DELETE("/v1beta/cachedContents/:cacheID", ok)
			router.GET("/v1beta/cachedContents-evil", ok)
			router.POST("/v1beta/models/*modelAction", ok)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("x-goog-api-key", "sk-test")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}
}
