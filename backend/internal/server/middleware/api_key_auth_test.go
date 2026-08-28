//go:build unit

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthRejectsOversizedCredentialsBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		calls.Add(1)
		return nil, service.ErrAPIKeyNotFound
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, cfg)

	for _, headers := range []map[string]string{
		{"x-api-key": strings.Repeat("x", service.MaxAPIKeyCredentialBytes+1)},
		{"Authorization": "Bearer " + strings.Repeat("x", service.MaxAPIKeyCredentialBytes+1)},
		{"Authorization": strings.Repeat("x", maxAPIKeyAuthorizationHeaderBytes+1)},
	} {
		r := gin.New()
		r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, cfg)))
		r.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
	require.Zero(t, calls.Load())
}

func TestAPIKeyAuthSetsGroupContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       101,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformAnthropic,
		Hydrated: true,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		userIDFromCtx, ok := c.Request.Context().Value(ctxkey.UserID).(int64)
		if !ok || userIDFromCtx != user.ID {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthRejectsExclusiveGroupWhenUserNoLongerAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:          202,
		Name:        "exclusive",
		Status:      service.StatusActive,
		IsExclusive: true,
		Hydrated:    true,
	}
	user := &service.User{
		ID:            7,
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Concurrency:   3,
		AllowedGroups: []int64{},
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "GROUP_NOT_ALLOWED")
}

func TestAPIKeyAuthOverwritesInvalidContextGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       101,
		Name:     "g1",
		Status:   service.StatusActive,
		Platform: service.PlatformAnthropic,
		Hydrated: true,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "test-key",
		Status: service.StatusActive,
		User:   user,
		Group:  group,
	}
	apiKey.GroupID = &group.ID

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))

	invalidGroup := &service.Group{
		ID:       group.ID,
		Platform: group.Platform,
		Status:   group.Status,
	}
	router.GET("/t", func(c *gin.Context) {
		groupFromCtx, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		if !ok || groupFromCtx == nil || groupFromCtx.ID != group.ID || !groupFromCtx.Hydrated || groupFromCtx == invalidGroup {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, invalidGroup))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthRejectsUnavailableGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(101)
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}

	tests := []struct {
		name       string
		group      *service.Group
		wantStatus int
		wantCode   string
		wantMarked bool
		wantReject IngressRejectReason
	}{
		{
			name: "active group passes",
			group: &service.Group{
				ID:       groupID,
				Name:     "active",
				Status:   service.StatusActive,
				Platform: service.PlatformAnthropic,
				Hydrated: true,
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "disabled group is forbidden",
			group: &service.Group{
				ID:       groupID,
				Name:     "disabled",
				Status:   service.StatusDisabled,
				Platform: service.PlatformAnthropic,
				Hydrated: true,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "GROUP_DISABLED",
			wantMarked: true,
			wantReject: IngressRejectGroupDisabled,
		},
		{
			name: "deleted status group is forbidden",
			group: &service.Group{
				ID:       groupID,
				Name:     "deleted",
				Status:   "deleted",
				Platform: service.PlatformAnthropic,
				Hydrated: true,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "GROUP_DELETED",
			wantMarked: true,
			wantReject: IngressRejectGroupDeleted,
		},
		{
			name:       "missing group edge is forbidden",
			group:      nil,
			wantStatus: http.StatusForbidden,
			wantCode:   "GROUP_DELETED",
			wantMarked: true,
			wantReject: IngressRejectGroupDeleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := &service.APIKey{
				ID:      100,
				UserID:  user.ID,
				GroupID: &groupID,
				Key:     "test-key",
				Status:  service.StatusActive,
				User:    user,
				Group:   tt.group,
			}
			apiKeyRepo := &stubApiKeyRepo{
				getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
					if key != apiKey.Key {
						return nil, service.ErrAPIKeyNotFound
					}
					clone := *apiKey
					return &clone, nil
				},
			}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
			router := gin.New()
			var markedBusinessLimited bool
			var businessLimitedReason string
			var rejectReason IngressRejectReason
			var rejected bool
			router.Use(func(c *gin.Context) {
				c.Next()
				markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
				rejectReason, rejected = GetIngressRejectReason(c)
				if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
					businessLimitedReason, _ = v.(string)
				}
			})
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
			router.GET("/t", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			req.Header.Set("x-api-key", apiKey.Key)
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantCode != "" {
				require.Contains(t, w.Body.String(), tt.wantCode)
			}
			require.Equal(t, tt.wantMarked, markedBusinessLimited)
			require.Equal(t, tt.wantReject != "", rejected)
			require.Equal(t, tt.wantReject, rejectReason)
			if tt.wantMarked {
				require.Equal(t, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable, businessLimitedReason)
			}
		})
	}
}

func TestAPIKeyAuthMarksOnlyExpectedIngressRejections(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		path       string
		key        string
		authHeader string
		repoErr    error
		wantStatus int
		wantCode   string
		wantReason IngressRejectReason
	}{
		{
			name:       "query key deprecated",
			path:       "/t?key=legacy",
			wantStatus: http.StatusBadRequest,
			wantCode:   "api_key_in_query_deprecated",
			wantReason: IngressRejectQueryAPIKeyDeprecated,
		},
		{
			name:       "missing key",
			path:       "/t",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "API_KEY_REQUIRED",
			wantReason: IngressRejectAPIKeyRequired,
		},
		{
			name:       "malformed authorization",
			path:       "/t",
			authHeader: "Basic not-a-bearer-key",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "API_KEY_REQUIRED",
			wantReason: IngressRejectInvalidAPIKey,
		},
		{
			name:       "oversized key",
			path:       "/t",
			key:        strings.Repeat("x", service.MaxAPIKeyCredentialBytes+1),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_API_KEY",
			wantReason: IngressRejectInvalidAPIKey,
		},
		{
			name:       "invalid key",
			path:       "/t",
			key:        "invalid",
			repoErr:    service.ErrAPIKeyNotFound,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_API_KEY",
			wantReason: IngressRejectInvalidAPIKey,
		},
		{
			name:       "repository failure remains operational error",
			path:       "/t",
			key:        "valid-shape",
			repoErr:    errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:       "auth lookup bulkhead rejection is an admission rejection",
			path:       "/t",
			key:        "valid-shape",
			repoErr:    service.ErrAPIKeyAuthOverloaded,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "API_KEY_AUTH_OVERLOADED",
			wantReason: IngressRejectAPIKeyAuthOverloaded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				return nil, tt.repoErr
			}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, cfg)
			router := gin.New()
			var reason IngressRejectReason
			var rejected bool
			router.Use(func(c *gin.Context) {
				c.Next()
				reason, rejected = GetIngressRejectReason(c)
			})
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
			router.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.key != "" {
				req.Header.Set("x-api-key", tt.key)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Contains(t, w.Body.String(), tt.wantCode)
			require.Equal(t, tt.wantReason != "", rejected)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestAPIKeyAuthSetsOpsFallbackKeyOnEarlyAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(101)
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:      100,
		UserID:  user.ID,
		GroupID: &groupID,
		Key:     "test-key",
		Status:  service.StatusActive,
		User:    user,
		Group: &service.Group{
			ID:       groupID,
			Name:     "disabled",
			Status:   service.StatusDisabled,
			Platform: service.PlatformAnthropic,
			Hydrated: true,
		},
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)

	router := gin.New()
	var fallback *service.APIKey
	var fallbackOK bool
	router.Use(func(c *gin.Context) {
		c.Next()
		fallback, fallbackOK = GetOpsFallbackAPIKey(c)
	})
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	// 分组停用 → 早退中断，但 ops fallback key 仍应写入，含 user/group/platform。
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "GROUP_DISABLED")
	require.True(t, fallbackOK, "鉴权早退时也应写入 ops fallback api key")
	require.NotNil(t, fallback)
	require.Equal(t, apiKey.ID, fallback.ID)
	require.NotNil(t, fallback.User)
	require.Equal(t, user.ID, fallback.User.ID)
	require.NotNil(t, fallback.GroupID)
	require.Equal(t, groupID, *fallback.GroupID)
	require.NotNil(t, fallback.Group)
	require.Equal(t, service.PlatformAnthropic, fallback.Group.Platform)
}

func TestAPIKeyAuthGoogleSetsOpsFallbackKeyOnEarlyAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(202)
	user := &service.User{
		ID:          9,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:      200,
		UserID:  user.ID,
		GroupID: &groupID,
		Key:     "g-key",
		Status:  service.StatusActive,
		User:    user,
		Group: &service.Group{
			ID:       groupID,
			Name:     "disabled",
			Status:   service.StatusDisabled,
			Platform: service.PlatformGemini,
			Hydrated: true,
		},
	}
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)

	router := gin.New()
	var fallback *service.APIKey
	var fallbackOK bool
	router.Use(func(c *gin.Context) {
		c.Next()
		fallback, fallbackOK = GetOpsFallbackAPIKey(c)
	})
	router.Use(gin.HandlerFunc(APIKeyAuthGoogle(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-goog-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.True(t, fallbackOK, "Google 鉴权早退时也应写入 ops fallback api key")
	require.NotNil(t, fallback)
	require.Equal(t, apiKey.ID, fallback.ID)
	require.NotNil(t, fallback.User)
	require.Equal(t, user.ID, fallback.User.ID)
}

func TestRequireGroupAssignmentMarksUngroupedKeyBusinessLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)

	settingService := service.NewSettingService(fakeSettingRepo{
		values: map[string]string{
			service.SettingKeyAllowUngroupedKeyScheduling: "false",
		},
	}, &config.Config{})
	apiKey := &service.APIKey{
		ID:     100,
		Key:    "ungrouped-key",
		Status: service.StatusActive,
	}

	router := gin.New()
	var markedBusinessLimited bool
	var businessLimitedReason string
	var rejectReason IngressRejectReason
	var rejected bool
	router.Use(func(c *gin.Context) {
		c.Next()
		markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
		rejectReason, rejected = GetIngressRejectReason(c)
		if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
			businessLimitedReason, _ = v.(string)
		}
	})
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Next()
	})
	router.Use(RequireGroupAssignment(settingService, AnthropicErrorWriter))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "not assigned to any group")
	require.True(t, rejected)
	require.Equal(t, IngressRejectGroupUnassigned, rejectReason)
	require.True(t, markedBusinessLimited)
	require.Equal(t, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned, businessLimitedReason)
}

func TestAPIKeyAuthIPRestrictionUsesTrustedPathWhenSwitchDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"1.2.3.4"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.SetTrustForwardedIPForAPIKeyACL(false)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	var markedBusinessLimited bool
	var businessLimitedReason string
	router.Use(func(c *gin.Context) {
		c.Next()
		markedBusinessLimited = service.HasOpsClientBusinessLimited(c)
		if v, ok := c.Get(service.OpsClientBusinessLimitedReasonKey); ok {
			businessLimitedReason, _ = v.(string)
		}
	})
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "ACCESS_DENIED", "Access denied. Your IP is 9.9.9.9")
	require.True(t, markedBusinessLimited)
	require.Equal(t, service.OpsClientBusinessLimitedReasonIPRestriction, businessLimitedReason)
}

func TestAPIKeyAuthIPRestrictionIncludesClientIPForBlacklistDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPBlacklist: []string{"9.9.9.9"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "ACCESS_DENIED", "Access denied. Your IP is 9.9.9.9")
}

func TestAPIKeyAuthIPRestrictionUsesConfiguredTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"1.2.3.4"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.SetTrustForwardedIPForAPIKeyACL(false)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies([]string{"9.9.9.9"}))
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuthIPRestrictionUsesForwardedClientIPInDenialWhenTrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:          100,
		UserID:      user.ID,
		Key:         "test-key",
		Status:      service.StatusActive,
		User:        user,
		IPWhitelist: []string{"9.9.9.9"},
	}

	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.SetTrustForwardedIPForAPIKeyACL(false)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies([]string{"9.9.9.9"}))
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.RemoteAddr = "9.9.9.9:12345"
	req.Header.Set("x-api-key", apiKey.Key)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	requireAPIKeyAuthError(t, w, "ACCESS_DENIED", "Access denied. Your IP is 1.2.3.4")
}

func TestAPIKeyAuthTouchesLastUsedOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     100,
		UserID: user.ID,
		Key:    "touch-ok",
		Status: service.StatusActive,
		User:   user,
	}

	var touchedID int64
	var touchedAt time.Time
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchedID = id
			touchedAt = usedAt
			return nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, apiKey.ID, touchedID)
	require.False(t, touchedAt.IsZero(), "expected touch timestamp")
}

func TestAPIKeyAuthTouchLastUsedFailureDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          8,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     101,
		UserID: user.ID,
		Key:    "touch-fail",
		Status: service.StatusActive,
		User:   user,
	}

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return errors.New("db unavailable")
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "touch failure should not block request")
	require.Equal(t, 1, touchCalls)
}

func TestAPIKeyAuthTouchesLastUsedInStandardMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:          9,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	apiKey := &service.APIKey{
		ID:     102,
		UserID: user.ID,
		Key:    "touch-standard",
		Status: service.StatusActive,
		User:   user,
	}

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
			if key != apiKey.Key {
				return nil, service.ErrAPIKeyNotFound
			}
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(ctx context.Context, id int64, usedAt time.Time) error {
			touchCalls++
			return nil
		},
	}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, touchCalls)
}

func TestAPIKeyAuthBillingInfoSkipsBillingAndSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:       42,
		Name:     "subscription",
		Status:   service.StatusActive,
		Hydrated: true,
	}
	user := &service.User{
		ID:          7,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: 3,
	}
	expiredAt := time.Now().Add(-time.Hour)
	apiKey := &service.APIKey{
		ID:        100,
		UserID:    user.ID,
		Key:       "billing-info-auth-only",
		Status:    service.StatusAPIKeyQuotaExhausted,
		User:      user,
		GroupID:   &group.ID,
		Group:     group,
		Quota:     1,
		QuotaUsed: 1,
		ExpiresAt: &expiredAt,
	}

	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(context.Context, string) (*service.APIKey, error) {
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(context.Context, int64, time.Time) error {
			touchCalls++
			return nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Zero(t, touchCalls)
}

func TestAPIKeyAuthBillingInfoSkipsLastUsedInSimpleMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive}
	apiKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "billing-info-simple", Status: service.StatusActive, User: user}
	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(context.Context, string) (*service.APIKey, error) {
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(context.Context, int64, time.Time) error {
			touchCalls++
			return nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Zero(t, touchCalls)
}

func TestAPIKeyAuthUsageStillTouchesLastUsed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive}
	apiKey := &service.APIKey{ID: 100, UserID: user.ID, Key: "usage-touch", Status: service.StatusActive, User: user}
	touchCalls := 0
	apiKeyRepo := &stubApiKeyRepo{
		getByKey: func(context.Context, string) (*service.APIKey, error) {
			clone := *apiKey
			return &clone, nil
		},
		updateLastUsed: func(context.Context, int64, time.Time) error {
			touchCalls++
			return nil
		},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg)
	router := newAuthTestRouter(apiKeyService, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, touchCalls)
}

func TestAPIKeyAuthOpenAIQuotaErrorFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 11, Role: service.RoleUser, Status: service.StatusActive}
	group := &service.Group{ID: 8, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	apiKey := &service.APIKey{
		ID: 105, UserID: user.ID, Key: "openai-quota-exhausted", Status: service.StatusAPIKeyQuotaExhausted,
		User: user, Group: group, GroupID: &group.ID,
	}
	apiKeyRepo := &stubApiKeyRepo{getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
		if key != apiKey.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *apiKey
		userClone := *user
		clone.User = &userClone
		return &clone, nil
	}}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	router := newAuthTestRouter(service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg), cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	var response struct {
		Error struct {
			Message string  `json:"message"`
			Type    string  `json:"type"`
			Param   *string `json:"param"`
			Code    string  `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "API key 额度已用完", response.Error.Message)
	require.Equal(t, "insufficient_quota", response.Error.Type)
	require.Nil(t, response.Error.Param)
	require.Equal(t, "insufficient_quota", response.Error.Code)
}

func TestAPIKeyAuthQuotaErrorKeepsLegacyFormatOutsideResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{ID: 11, Role: service.RoleUser, Status: service.StatusActive}
	group := &service.Group{ID: 8, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	apiKey := &service.APIKey{
		ID: 105, UserID: user.ID, Key: "openai-quota-exhausted", Status: service.StatusAPIKeyQuotaExhausted,
		User: user, Group: group, GroupID: &group.ID,
	}
	apiKeyRepo := &stubApiKeyRepo{getByKey: func(ctx context.Context, key string) (*service.APIKey, error) {
		if key != apiKey.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *apiKey
		userClone := *user
		clone.User = &userClone
		return &clone, nil
	}}

	cfg := &config.Config{RunMode: config.RunModeStandard}
	router := newAuthTestRouter(service.NewAPIKeyService(apiKeyRepo, nil, nil, nil, nil, cfg), cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("x-api-key", apiKey.Key)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	requireAPIKeyAuthError(t, w, "API_KEY_QUOTA_EXHAUSTED", "API key 额度已用完")
}

func newAuthTestRouter(apiKeyService *service.APIKeyService, cfg *config.Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, cfg)))
	ok := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
	router.GET("/t", ok)
	router.POST("/v1/responses", ok)
	router.POST("/v1/messages", ok)
	router.GET("/v1/usage", ok)
	router.GET("/v1/sub2api/billing", ok)
	return router
}

func requireAPIKeyAuthError(t *testing.T, w *httptest.ResponseRecorder, code, message string) {
	t.Helper()

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, code, resp.Code)
	require.Equal(t, message, resp.Message)
}

type stubApiKeyRepo struct {
	getByKey       func(ctx context.Context, key string) (*service.APIKey, error)
	updateLastUsed func(ctx context.Context, id int64, usedAt time.Time) error
}

func (r *stubApiKeyRepo) Create(ctx context.Context, key *service.APIKey) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	return "", 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	if r.getByKey != nil {
		return r.getByKey(ctx, key)
	}
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) GetByKeyForAuth(ctx context.Context, key string) (*service.APIKey, error) {
	return r.GetByKey(ctx, key)
}

func (r *stubApiKeyRepo) Update(ctx context.Context, key *service.APIKey, _ service.APIKeyUpdateFields) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) Delete(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) DeleteWithAudit(ctx context.Context, id int64) error {
	return errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ExistsByKey(ctx context.Context, key string) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (r *stubApiKeyRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	return 0, errors.New("not implemented")
}

func (r *stubApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	if r.updateLastUsed != nil {
		return r.updateLastUsed(ctx, id, usedAt)
	}
	return nil
}

func (r *stubApiKeyRepo) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	return nil
}
func (r *stubApiKeyRepo) ResetRateLimitWindows(ctx context.Context, id int64) error {
	return nil
}
func (r *stubApiKeyRepo) GetRateLimitData(ctx context.Context, id int64) (*service.APIKeyRateLimitData, error) {
	return nil, nil
}

type fakeSettingRepo struct {
	values map[string]string
}

func (r fakeSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	return nil, errors.New("not implemented")
}

func (r fakeSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}

func (r fakeSettingRepo) Set(ctx context.Context, key, value string) error {
	return errors.New("not implemented")
}

func (r fakeSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r fakeSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	return errors.New("not implemented")
}

func (r fakeSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r fakeSettingRepo) Delete(ctx context.Context, key string) error {
	return errors.New("not implemented")
}
