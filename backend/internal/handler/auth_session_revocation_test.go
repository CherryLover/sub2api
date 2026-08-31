//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// userHandlerRefreshTokenCacheStub 是 AuthService 的最小 refresh token 缓存桩。
//
// 它原本住在 user_handler_test.go 里，已经被裁剪工程连带删掉两次（先是「整删邮件体系」，
// 再是「删掉解绑第三方登录的接口」），每次都让 unit tag 下的 handler 包编译不过。
// 现在把它挪到唯一的使用方这里，跟着用例一起存亡，避免第三次。
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

func TestAuthHandlerRevokeAllSessionsInvalidatesAccessTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{
			ID:           29,
			Email:        "session@example.com",
			Username:     "session-user",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			TokenVersion: 7,
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
	handler := &AuthHandler{authService: authService}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/revoke-all-sessions", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 29})

	handler.RevokeAllSessions(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, []int64{29}, refreshTokenCache.revokedUserIDs)
	// users 表没有 token_version 列（见 resolvedTokenVersion：JWT 里的值由
	// email+password_hash 指纹推导），所以自增 TokenVersion 只停留在内存里。
	// 此前紧跟其后的整行 Update 不写任何有效数据，却会用旧快照覆盖并发写入的列，
	// 已移除。会话撤销由上面的 refresh session 清理承担。
	require.Equal(t, int64(7), repo.user.TokenVersion)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "All sessions have been revoked. Please log in again.", resp.Data.Message)
}
