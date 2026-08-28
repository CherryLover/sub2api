//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userHandlerRepoStub struct {
	user       *service.User
	identities []service.UserAuthIdentityRecord
	unbound    []string
}

func (s *userHandlerRepoStub) Create(context.Context, *service.User) error { return nil }
func (s *userHandlerRepoStub) CreateWithEmailAliasGuard(context.Context, *service.User) error {
	return nil
}
func (s *userHandlerRepoStub) GetByID(context.Context, int64) (*service.User, error) {
	cloned := *s.user
	return &cloned, nil
}
func (s *userHandlerRepoStub) GetByEmail(context.Context, string) (*service.User, error) {
	cloned := *s.user
	return &cloned, nil
}
func (s *userHandlerRepoStub) GetFirstAdmin(context.Context) (*service.User, error) {
	cloned := *s.user
	return &cloned, nil
}
func (s *userHandlerRepoStub) Update(_ context.Context, user *service.User, _ service.UserUpdateFields) error {
	cloned := *user
	s.user = &cloned
	return nil
}
func (s *userHandlerRepoStub) Delete(context.Context, int64) error { return nil }
func (s *userHandlerRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	if s.user == nil || s.user.AvatarURL == "" {
		return nil, nil
	}
	return &service.UserAvatar{
		StorageProvider: s.user.AvatarSource,
		URL:             s.user.AvatarURL,
		ContentType:     s.user.AvatarMIME,
		ByteSize:        s.user.AvatarByteSize,
		SHA256:          s.user.AvatarSHA256,
	}, nil
}
func (s *userHandlerRepoStub) UpsertUserAvatar(_ context.Context, _ int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	s.user.AvatarURL = input.URL
	s.user.AvatarSource = input.StorageProvider
	s.user.AvatarMIME = input.ContentType
	s.user.AvatarByteSize = input.ByteSize
	s.user.AvatarSHA256 = input.SHA256
	return &service.UserAvatar{
		StorageProvider: input.StorageProvider,
		URL:             input.URL,
		ContentType:     input.ContentType,
		ByteSize:        input.ByteSize,
		SHA256:          input.SHA256,
	}, nil
}
func (s *userHandlerRepoStub) DeleteUserAvatar(context.Context, int64) error {
	s.user.AvatarURL = ""
	s.user.AvatarSource = ""
	s.user.AvatarMIME = ""
	s.user.AvatarByteSize = 0
	s.user.AvatarSHA256 = ""
	return nil
}
func (s *userHandlerRepoStub) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userHandlerRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userHandlerRepoStub) UpdateConcurrency(context.Context, int64, int) error { return nil }

// userHandlerRefreshTokenCacheStub 是 AuthService 的最小 refresh token 缓存桩。
// 它原本住在本文件里，被「整删邮件体系」那次提交连带删掉，但两处用例还在引用，
// 导致 unit tag 下的 handler 包编译不过；这里按原样补回。
type userHandlerRefreshTokenCacheStub struct {
	revokedUserIDs []int64
}

func (s *userHandlerRefreshTokenCacheStub) StoreRefreshToken(context.Context, string, *service.RefreshTokenData, time.Duration) error {
	return nil
}

func (s *userHandlerRefreshTokenCacheStub) GetRefreshToken(context.Context, string) (*service.RefreshTokenData, error) {
	return nil, service.ErrRefreshTokenNotFound
}

func (s *userHandlerRefreshTokenCacheStub) DeleteRefreshToken(context.Context, string) error {
	return nil
}

func (s *userHandlerRefreshTokenCacheStub) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	s.revokedUserIDs = append(s.revokedUserIDs, userID)
	return nil
}

func (s *userHandlerRefreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
}

func (s *userHandlerRefreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *userHandlerRefreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *userHandlerRefreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *userHandlerRefreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}

func (s *userHandlerRefreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s *userHandlerRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (s *userHandlerRepoStub) ExistsByEmailAlias(context.Context, string) (bool, error) {
	return false, nil
}
func (s *userHandlerRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *userHandlerRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}
func (s *userHandlerRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (s *userHandlerRepoStub) UpdateUserLastActiveAt(_ context.Context, _ int64, activeAt time.Time) error {
	if s.user != nil {
		s.user.LastActiveAt = &activeAt
	}
	return nil
}
func (s *userHandlerRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *userHandlerRepoStub) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (s *userHandlerRepoStub) EnableTotp(context.Context, int64) error                { return nil }
func (s *userHandlerRepoStub) DisableTotp(context.Context, int64) error               { return nil }
func (s *userHandlerRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	return s.GetByID(ctx, id)
}
func (s *userHandlerRepoStub) ListUserAuthIdentities(context.Context, int64) ([]service.UserAuthIdentityRecord, error) {
	out := make([]service.UserAuthIdentityRecord, len(s.identities))
	copy(out, s.identities)
	return out, nil
}
func (s *userHandlerRepoStub) UnbindUserAuthProvider(_ context.Context, _ int64, provider string) error {
	s.unbound = append(s.unbound, provider)
	filtered := s.identities[:0]
	for _, identity := range s.identities {
		if identity.ProviderType == provider {
			continue
		}
		filtered = append(filtered, identity)
	}
	s.identities = append([]service.UserAuthIdentityRecord(nil), filtered...)
	return nil
}

func TestUserHandlerUpdateProfileReturnsAvatarURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:       11,
			Email:    "handler-avatar@example.com",
			Username: "handler-avatar",
			Role:     service.RoleUser,
			Status:   service.StatusActive,
		},
	}
	handler := NewUserHandler(service.NewUserService(repo, nil, nil), nil, nil)

	body := []byte(`{"avatar_url":"https://cdn.example.com/avatar.png"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/user", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 11})

	handler.UpdateProfile(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			AvatarURL string `json:"avatar_url"`
			Username  string `json:"username"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "https://cdn.example.com/avatar.png", resp.Data.AvatarURL)
	require.Equal(t, "handler-avatar", resp.Data.Username)
}

func TestUserHandlerUnbindIdentityReturnsUpdatedProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:       21,
			Email:    "identity@example.com",
			Username: "identity-user",
			Role:     service.RoleUser,
			Status:   service.StatusActive,
		},
		identities: []service.UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "identity@example.com",
			},
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-21",
				Metadata: map[string]any{
					"username": "linuxdo-handle",
				},
			},
		},
	}
	handler := NewUserHandler(service.NewUserService(repo, nil, nil), nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/user/account-bindings/linuxdo", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 21})
	c.Params = gin.Params{{Key: "provider", Value: "linuxdo"}}

	handler.UnbindIdentity(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []string{"linuxdo"}, repo.unbound)

	var resp struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	// 绑定摘要已收敛为只含 email：解绑成功后响应中不再出现第三方 provider 条目。
	authBindings, ok := resp.Data["auth_bindings"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, authBindings, "email")
	require.NotContains(t, authBindings, "linuxdo")
}

func TestUserHandlerUnbindIdentityRevokesAllUserSessionsWhenAuthServiceConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:           23,
			Email:        "identity@example.com",
			Username:     "identity-user",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			TokenVersion: 4,
		},
		identities: []service.UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "identity@example.com",
			},
			{
				ProviderType:    "linuxdo",
				ProviderKey:     "linuxdo",
				ProviderSubject: "linuxdo-subject-23",
			},
		},
	}
	refreshTokenCache := &userHandlerRefreshTokenCacheStub{}
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
		},
	}
	authService := service.NewAuthService(nil, repo, refreshTokenCache, cfg, nil)
	handler := NewUserHandler(service.NewUserService(repo, nil, nil), authService, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/user/account-bindings/linuxdo", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 23})
	c.Params = gin.Params{{Key: "provider", Value: "linuxdo"}}

	handler.UnbindIdentity(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{23}, refreshTokenCache.revokedUserIDs)
	// 撤销依赖的是 refresh session 清理，而不是 token_version：users 表没有这一列
	// （见 resolvedTokenVersion，实际值由 email+password_hash 指纹推导），
	// 所以此前"自增 TokenVersion 再整行写回"不持久化任何东西，
	// 却会用旧快照覆盖并发写入的列。这里断言用户行未被改写。
	require.Equal(t, int64(4), repo.user.TokenVersion)
}

func TestUserHandlerUnbindIdentityDoesNotRevokeSessionsWhenNothingWasUnbound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:           24,
			Email:        "identity@example.com",
			Username:     "identity-user",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			TokenVersion: 4,
		},
		identities: []service.UserAuthIdentityRecord{
			{
				ProviderType:    "email",
				ProviderKey:     "email",
				ProviderSubject: "identity@example.com",
			},
		},
	}
	refreshTokenCache := &userHandlerRefreshTokenCacheStub{}
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
		},
	}
	authService := service.NewAuthService(nil, repo, refreshTokenCache, cfg, nil)
	handler := NewUserHandler(service.NewUserService(repo, nil, nil), authService, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/user/account-bindings/linuxdo", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 24})
	c.Params = gin.Params{{Key: "provider", Value: "linuxdo"}}

	handler.UnbindIdentity(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, repo.unbound)
	require.Empty(t, refreshTokenCache.revokedUserIDs)
	require.Equal(t, int64(4), repo.user.TokenVersion)
}
