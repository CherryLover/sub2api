//go:build unit

package server

// 裁剪面门禁测试（防回归护栏）。
//
// 本项目按"单管理员内部部署"裁剪掉了支付/订单、卡密（redeem）、优惠码
// （promo）、返佣（affiliate）与六种第三方 OAuth 登录（LinuxDo/微信/钉钉/
// OIDC/GitHub/Google），并将多用户注册默认关闭。这里用生产 registerRoutes
// 装配完整路由表后断言：
//
//  1. 被裁剪的 SaaS 面路由不存在（精确路径 + 前缀兜底 + 请求级 404）；
//  2. 默认设置下 POST /api/v1/auth/register 被注册开关拒绝；
//  3. GET /api/v1/settings/public 不再泄露 payment_enabled 键；
//  4. 保留面（login/2fa/passkey 登录）完好，防止误删。
//
// 将来任何人把这些面加回 router.go / routes/*.go，CI 会立即变红。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// gateSettingRepo 门禁测试用的内存设置仓库：完整实现 SettingRepository，
// 让 InitializeDefaultSettings 的种子写入与运行期读取都走真实语义。
type gateSettingRepo struct {
	values map[string]string
}

func newGateSettingRepo() *gateSettingRepo {
	return &gateSettingRepo{values: map[string]string{}}
}

func (r *gateSettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	v, ok := r.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: v}, nil
}

func (r *gateSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	v, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return v, nil
}

func (r *gateSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *gateSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := r.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (r *gateSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *gateSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *gateSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

// newTrimmedSurfaceRouter 用生产 registerRoutes 装配完整面板路由表。
// 设置仓库先经 InitializeDefaultSettings 种子，模拟全新部署的默认设置。
func newTrimmedSurfaceRouter(t *testing.T) (*gin.Engine, *gateSettingRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	repo := newGateSettingRepo()
	cfg := &config.Config{}
	cfg.Pricing.DataDir = t.TempDir()

	settingService := service.NewSettingService(repo, cfg)
	require.NoError(t, settingService.InitializeDefaultSettings(context.Background()))

	authService := service.NewAuthService(nil, nil, nil, cfg, settingService, nil, nil, nil, nil, nil)

	handlers := &handler.Handlers{
		Auth:          handler.NewAuthHandler(cfg, authService, nil, settingService, nil, nil),
		Setting:       handler.NewSettingHandler(settingService, "test"),
		Admin:         &handler.AdminHandlers{},
		Gateway:       &handler.GatewayHandler{},
		OpenAIGateway: &handler.OpenAIGatewayHandler{},
		AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	}

	pass := func(c *gin.Context) { c.Next() }
	r := gin.New()
	registerRoutes(
		r,
		handlers,
		middleware2.JWTAuthMiddleware(pass),
		middleware2.OptionalJWTAuthMiddleware(pass),
		middleware2.AdminAuthMiddleware(pass),
		middleware2.APIKeyAuthMiddleware(pass),
		middleware2.AuditLogMiddleware(pass),
		middleware2.StepUpAuthMiddleware(pass),
		nil, // apiKeyService
		nil, // subscriptionService
		nil, // opsService
		settingService,
		nil, // compositeResolver
		cfg,
		rdb,
	)
	return r, repo
}

func TestTrimmedSaaSRoutesAreAbsent(t *testing.T) {
	router, _ := newTrimmedSurfaceRouter(t)

	paths := make(map[string]struct{})
	for _, route := range router.Routes() {
		paths[route.Path] = struct{}{}
	}

	// 代表性精确路径：任何 HTTP 方法下都不允许重新注册。
	absentPaths := []string{
		// 支付/订单
		"/api/v1/payment/config",
		"/api/v1/payment/orders",
		"/api/v1/payment/orders/:id",
		"/api/v1/payment/webhook/easypay",
		"/api/v1/admin/payment/dashboard",
		"/api/v1/admin/payment/config",
		// 卡密
		"/api/v1/redeem",
		"/api/v1/redeem/history",
		"/api/v1/admin/redeem-codes",
		// 优惠码
		"/api/v1/admin/promo-codes",
		"/api/v1/auth/validate-promo-code",
		// 邀请码/返佣
		"/api/v1/auth/validate-invitation-code",
		"/api/v1/user/aff",
		"/api/v1/user/aff/transfer",
		"/api/v1/admin/affiliates/invites",
		// 余额流水（随支付面一并裁剪）
		"/api/v1/admin/users/:id/balance-history",
		// 第三方 OAuth 登录（每个 provider 至少一条代表路由）
		"/api/v1/auth/oauth/linuxdo/start",
		"/api/v1/auth/oauth/github/callback",
		"/api/v1/auth/oauth/google/callback",
		"/api/v1/auth/oauth/wechat/callback",
		"/api/v1/auth/oauth/oidc/start",
		"/api/v1/auth/oauth/dingtalk/start",
		"/api/v1/auth/oauth/pending/exchange",
		// 第三方绑定启动（OAuth 登录删除后已成死胡同端点，随 WP4 一并移除）
		"/api/v1/user/auth-identities/bind/start",
		// 应用内更新检查/在线升级/回滚（内部部署由镜像或部署脚本升级）
		"/api/v1/admin/system/check-updates",
		"/api/v1/admin/system/rollback-versions",
		"/api/v1/admin/system/update",
		"/api/v1/admin/system/rollback",
	}
	for _, path := range absentPaths {
		_, exists := paths[path]
		require.Falsef(t, exists, "已裁剪路由 %s 不应重新注册", path)
	}

	// 前缀级兜底：换个参数名或子路径加回来同样拦截。
	// 注意 /api/v1/auth/oauth 仅覆盖用户登录 OAuth；管理端上游账号 OAuth
	// （/api/v1/admin/.../oauth/...）是保留面，不在此列。
	forbiddenPrefixes := []string{
		"/api/v1/payment",
		"/api/v1/admin/payment",
		"/api/v1/redeem",
		"/api/v1/user/aff",
		"/api/v1/admin/redeem-codes",
		"/api/v1/admin/promo-codes",
		"/api/v1/admin/affiliates",
		"/api/v1/auth/oauth",
	}
	for path := range paths {
		for _, prefix := range forbiddenPrefixes {
			require.Falsef(t, strings.HasPrefix(path, prefix), "路由 %s 落入已裁剪面前缀 %s", path, prefix)
		}
	}

	// 请求级兜底：防止将来通过 NoRoute/通配符路由复活这些端点。
	for _, probe := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/payment/config"},
		{http.MethodPost, "/api/v1/payment/webhook/easypay"},
		{http.MethodPost, "/api/v1/redeem"},
		{http.MethodGet, "/api/v1/user/aff"},
		{http.MethodGet, "/api/v1/admin/payment/dashboard"},
		{http.MethodGet, "/api/v1/auth/oauth/linuxdo/start"},
		{http.MethodGet, "/api/v1/admin/system/check-updates"},
		{http.MethodPost, "/api/v1/admin/system/update"},
		{http.MethodPost, "/api/v1/admin/system/rollback"},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(probe.method, probe.path, nil)
		req.RemoteAddr = "203.0.113.10:12345"
		router.ServeHTTP(w, req)
		require.Equalf(t, http.StatusNotFound, w.Code, "%s %s 应返回 404", probe.method, probe.path)
	}
}

func TestRetainedAuthSurfaceStillRegistered(t *testing.T) {
	router, _ := newTrimmedSurfaceRouter(t)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, want := range []string{
		"POST /api/v1/auth/register", // 路由保留，默认设置下由注册开关拒绝
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/login/2fa",
		"POST /api/v1/auth/passkey/login/begin",
	} {
		_, exists := routes[want]
		require.Truef(t, exists, "保留面路由 %s 不应被误删", want)
	}
}

// TestRetainedSystemSurfaceStillRegistered 守住裁掉自更新后 system 段该留的两条：
// 版本号展示（侧边栏用）与服务重启（运维用）。
func TestRetainedSystemSurfaceStillRegistered(t *testing.T) {
	router, _ := newTrimmedSurfaceRouter(t)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, want := range []string{
		"GET /api/v1/admin/system/version",
		"POST /api/v1/admin/system/restart",
	} {
		_, exists := routes[want]
		require.Truef(t, exists, "保留面路由 %s 不应被误删", want)
	}
}

func TestRegisterRejectedUnderDefaultSettings(t *testing.T) {
	router, repo := newTrimmedSurfaceRouter(t)

	// 种子层面：registration_enabled 默认必须是 false（多用户默认禁用）。
	require.Equal(t, "false", repo.values[service.SettingKeyRegistrationEnabled])

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"email":"user@example.com","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:12345"
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "REGISTRATION_DISABLED")
}

// TestRegistrationDefaultsClosedWithoutSeededSettings 覆盖全新库尚未种子
// 默认设置的场景：registration_enabled 行缺失时注册同样 fail-closed。
func TestRegistrationDefaultsClosedWithoutSeededSettings(t *testing.T) {
	settingService := service.NewSettingService(newGateSettingRepo(), &config.Config{})
	require.False(t, settingService.IsRegistrationEnabled(context.Background()))
}

func TestPublicSettingsHasNoPaymentKey(t *testing.T) {
	router, _ := newTrimmedSurfaceRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int                        `json:"code"`
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data)
	require.NotContains(t, resp.Data, "payment_enabled")
	// 六种第三方 OAuth 登录的公开开关键随 WP4 清扫一并移除，不许回流。
	for _, key := range []string{
		"linuxdo_oauth_enabled",
		"dingtalk_oauth_enabled",
		"wechat_oauth_enabled",
		"oidc_oauth_enabled",
		"github_oauth_enabled",
		"google_oauth_enabled",
	} {
		require.NotContainsf(t, resp.Data, key, "公开设置不应再包含 OAuth 登录开关键 %s", key)
	}
	// promo/invitation/affiliate/purchase 的惰性设置键同样不许回流。
	for _, key := range []string{
		"promo_code_enabled",
		"invitation_code_enabled",
		"affiliate_enabled",
		"purchase_subscription_enabled",
		"purchase_subscription_url",
	} {
		require.NotContainsf(t, resp.Data, key, "公开设置不应再包含已裁剪功能键 %s", key)
	}
	// 公开设置同时应体现注册默认关闭。
	require.JSONEq(t, "false", string(resp.Data["registration_enabled"]))
}
