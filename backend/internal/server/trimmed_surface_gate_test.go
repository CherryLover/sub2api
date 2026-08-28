//go:build unit

package server

// 裁剪面门禁测试（防回归护栏）。
//
// 本项目按"单管理员内部部署"裁剪掉了支付/订单、卡密（redeem）、优惠码
// （promo）、返佣（affiliate）、六种第三方 OAuth 登录（LinuxDo/微信/钉钉/
// OIDC/GitHub/Google），以及注册体系与人机验证；批次 2 又整套删掉了管理员
// 合规确认（/admin/compliance + AdminComplianceGuard）、登录条款与自定义
// 菜单页面（/pages）。这里用生产 registerRoutes 装配完整路由表后断言：
//
//  1. 被裁剪的 SaaS 面路由不存在（精确路径 + 前缀兜底 + 请求级 404）；
//  2. 注册体系的两条入口（register / send-verify-code）与邮件体系
//     （forgot/reset-password、email-unsubscribe、SMTP 自测、邮件模板、
//     通知邮箱、邮箱绑定验证码、TOTP 邮箱验证码）路由整体缺席；
//  3. GET /api/v1/settings/public 不再泄露 payment_enabled、人机验证键，
//     以及登录条款与已裁剪的站点/表格/自定义菜单设置键；
//  4. 保留面（login/2fa/passkey 登录/refresh/logout）完好，防止误删。
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

	authService := service.NewAuthService(nil, nil, nil, cfg, settingService, nil)

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
		// 注册体系（B3）：注册入口与注册邮箱验证码发送整体移除
		"/api/v1/auth/register",
		"/api/v1/auth/send-verify-code",
		// 邮件体系（B5 方案 A）：忘记密码 / 重置密码 / 退订 / SMTP 自测 / 模板编辑
		"/api/v1/auth/forgot-password",
		"/api/v1/auth/reset-password",
		"/api/v1/settings/email-unsubscribe",
		"/api/v1/admin/settings/test-smtp",
		"/api/v1/admin/settings/send-test-email",
		"/api/v1/admin/settings/email-templates",
		"/api/v1/admin/settings/email-template-preview",
		"/api/v1/admin/settings/email-templates/:event/:locale",
		"/api/v1/admin/settings/email-templates/:event/:locale/restore-official",
		"/api/v1/admin/ops/email-notification/config",
		// 通知邮箱 / 邮箱绑定验证码 / TOTP 邮箱验证码
		"/api/v1/user/notify-email",
		"/api/v1/user/notify-email/send-code",
		"/api/v1/user/notify-email/verify",
		"/api/v1/user/notify-email/toggle",
		"/api/v1/user/account-bindings/email",
		"/api/v1/user/account-bindings/email/send-code",
		"/api/v1/user/totp/send-code",
		// 应用内更新检查/在线升级/回滚（内部部署由镜像或部署脚本升级）
		"/api/v1/admin/system/check-updates",
		"/api/v1/admin/system/rollback-versions",
		"/api/v1/admin/system/update",
		"/api/v1/admin/system/rollback",
		// 管理员部署与运营合规确认（登录后先弹"我已同意"的那一套）
		"/api/v1/admin/compliance",
		"/api/v1/admin/compliance/accept",
		// 自定义菜单页面（iframe/Markdown 映射页）
		"/api/v1/pages",
		"/api/v1/pages/:slug",
		"/api/v1/pages/:slug/images/*filename",
		// 内容安全审计（批次 3）：管理端 /admin/risk-control 全组下线。
		// 提示词审计 /admin/prompt-audit 是保留面，见
		// TestRetainedPromptAuditSurfaceStillRegistered。
		"/api/v1/admin/risk-control/config",
		"/api/v1/admin/risk-control/status",
		"/api/v1/admin/risk-control/logs",
		"/api/v1/admin/risk-control/api-keys/test",
		"/api/v1/admin/risk-control/users/:user_id/unban",
		"/api/v1/admin/risk-control/hashes",
		"/api/v1/admin/risk-control/hashes/all",
		// 批量生图（批次 3）：网关十条 /v1/images/batches* 端点整体移除。
		// 普通生图 /v1/images/generations、/v1/images/edits 与异步生图
		// /v1/images/*/async 是保留面，见 TestRetainedImageSurfaceStillRegistered。
		"/v1/images/batches",
		"/v1/images/batches/models",
		"/v1/images/batches/:id",
		"/v1/images/batches/:id/items",
		"/v1/images/batches/:id/items/:custom_id/content",
		"/v1/images/batches/:id/download",
		"/v1/images/batches/:id/cancel",
		"/v1/images/batches/:id/outputs",
		// 模型广场（批次 3）：公开的分组/模型定价橱窗整体下线。
		"/api/v1/model-plaza",
		// 公告体系（批次 3）：用户侧公告列表/已读回执与管理端公告 CRUD 整体下线。
		"/api/v1/announcements",
		"/api/v1/announcements/:id/read",
		"/api/v1/admin/announcements",
		"/api/v1/admin/announcements/:id",
		"/api/v1/admin/announcements/:id/read-status",
	}
	for _, path := range absentPaths {
		_, exists := paths[path]
		require.Falsef(t, exists, "已裁剪路由 %s 不应重新注册", path)
	}

	// 前缀级兜底：换个参数名或子路径加回来同样拦截。
	// 注意 /api/v1/auth/oauth 仅覆盖用户登录 OAuth；管理端上游账号 OAuth
	// （/api/v1/admin/.../oauth/...）是保留面，不在此列。
	forbiddenPrefixes := []string{
		"/v1/images/batches",
		"/api/v1/admin/risk-control",
		"/api/v1/payment",
		"/api/v1/admin/payment",
		"/api/v1/redeem",
		"/api/v1/user/aff",
		"/api/v1/admin/redeem-codes",
		"/api/v1/admin/promo-codes",
		"/api/v1/admin/affiliates",
		"/api/v1/auth/oauth",
		"/api/v1/admin/compliance",
		"/api/v1/pages",
		"/api/v1/announcements",
		"/api/v1/admin/announcements",
		"/api/v1/model-plaza",
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
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/send-verify-code"},
		{http.MethodPost, "/api/v1/auth/forgot-password"},
		{http.MethodPost, "/api/v1/auth/reset-password"},
		{http.MethodGet, "/api/v1/settings/email-unsubscribe"},
		{http.MethodPost, "/api/v1/admin/settings/test-smtp"},
		{http.MethodGet, "/api/v1/admin/settings/email-templates"},
		{http.MethodPost, "/api/v1/user/notify-email/send-code"},
		{http.MethodPost, "/api/v1/user/account-bindings/email/send-code"},
		{http.MethodPost, "/api/v1/user/totp/send-code"},
		{http.MethodGet, "/api/v1/admin/system/check-updates"},
		{http.MethodPost, "/api/v1/admin/system/update"},
		{http.MethodPost, "/api/v1/admin/system/rollback"},
		{http.MethodGet, "/api/v1/model-plaza"},
		{http.MethodGet, "/api/v1/announcements"},
		{http.MethodGet, "/api/v1/admin/announcements"},
		{http.MethodGet, "/api/v1/admin/compliance"},
		{http.MethodPost, "/api/v1/admin/compliance/accept"},
		{http.MethodGet, "/api/v1/pages/help"},
		{http.MethodPost, "/v1/images/batches"},
		{http.MethodGet, "/v1/images/batches"},
		{http.MethodGet, "/v1/images/batches/models"},
		{http.MethodGet, "/v1/images/batches/imgbatch_1"},
		{http.MethodPost, "/v1/images/batches/imgbatch_1/cancel"},
		{http.MethodDelete, "/v1/images/batches/imgbatch_1"},
		{http.MethodGet, "/api/v1/admin/risk-control/config"},
		{http.MethodGet, "/api/v1/admin/risk-control/logs"},
		{http.MethodGet, "/api/v1/admin/risk-control/status"},
		{http.MethodPost, "/api/v1/admin/risk-control/api-keys/test"},
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
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/login/2fa",
		"POST /api/v1/auth/passkey/login/begin",
		"POST /api/v1/auth/passkey/login/finish",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
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

// TestRetainedImageSurfaceStillRegistered 批量生图删除后，普通生图与异步生图
// 必须原样保留 —— 这是两个不同功能，别一起误删。
func TestRetainedImageSurfaceStillRegistered(t *testing.T) {
	router, _ := newTrimmedSurfaceRouter(t)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, want := range []string{
		"POST /v1/images/generations",
		"POST /v1/images/edits",
		"POST /v1/images/generations/async",
		"POST /v1/images/edits/async",
		"GET /v1/images/tasks/:task_id",
	} {
		_, exists := routes[want]
		require.Truef(t, exists, "保留面路由 %s 不应被误删", want)
	}
}

// TestRetainedPromptAuditSurfaceStillRegistered 内容安全审计删除后，提示词审计
// 必须原样保留 —— 两者是不同功能，risk_control_enabled 现在只作为提示词审计的总开关。
func TestRetainedPromptAuditSurfaceStillRegistered(t *testing.T) {
	router, _ := newTrimmedSurfaceRouter(t)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, want := range []string{
		"GET /api/v1/admin/prompt-audit/config",
		"PUT /api/v1/admin/prompt-audit/config",
		"GET /api/v1/admin/prompt-audit/runtime",
		"GET /api/v1/admin/prompt-audit/events",
	} {
		_, exists := routes[want]
		require.Truef(t, exists, "保留面路由 %s 不应被误删", want)
	}
}

// TestPublicSettingsKeepsRiskControlSwitch risk_control_enabled 是提示词审计的
// 总开关，内容安全审计删除后仍必须出现在公开设置里，否则前端菜单与路由守卫会失效。
func TestPublicSettingsKeepsRiskControlSwitch(t *testing.T) {
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
	require.Contains(t, resp.Data, "risk_control_enabled")
	require.NotContains(t, resp.Data, "content_moderation_config")
}

// TestRegistrationSurfaceIsAbsent 注册体系已整体移除：入口不再是"被开关拒绝"
// 而是路由根本不存在，带合法载荷的请求也只能拿到 404。
func TestRegistrationSurfaceIsAbsent(t *testing.T) {
	router, repo := newTrimmedSurfaceRouter(t)

	// 种子层面：registration_enabled 已不再写入任何默认值。
	require.NotContains(t, repo.values, "registration_enabled")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"email":"user@example.com","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:12345"
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/send-verify-code",
		strings.NewReader(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:12345"
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
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
	// 注册体系（B3）与人机验证（B4）的公开设置键随之整体移除，不许回流。
	for _, key := range []string{
		"registration_enabled",
		"turnstile_enabled",
		"turnstile_site_key",
		"tencent_captcha_enabled",
		"tencent_captcha_app_id",
		"aliyun_captcha_enabled",
		"aliyun_captcha_scene_id",
	} {
		require.NotContainsf(t, resp.Data, key, "公开设置不应再包含已裁剪的注册/人机验证键 %s", key)
	}
	// 邮件体系（B5 方案 A）：SMTP 与邮件相关的公开开关键整体移除，不许回流。
	for _, key := range []string{
		"email_verify_enabled",
		"password_reset_enabled",
		"balance_low_notify_enabled",
		"balance_low_notify_threshold",
		"balance_low_notify_recharge_url",
		"account_quota_notify_enabled",
	} {
		require.NotContainsf(t, resp.Data, key, "公开设置不应再包含已裁剪的邮件相关键 %s", key)
	}
	// 批次 2 裁掉的登录条款与通用设置冗余项，公开设置里同样不许回流。
	for _, key := range []string{
		"login_agreement_enabled",
		"login_agreement_mode",
		"login_agreement_updated_at",
		"login_agreement_revision",
		"login_agreement_documents",
		"site_name",
		"site_subtitle",
		"site_logo",
		"api_base_url",
		"contact_info",
		"home_content",
		"compact_home_enabled",
		"hide_ccs_import_button",
		"table_default_page_size",
		"table_page_size_options",
		"custom_menu_items",
	} {
		require.NotContainsf(t, resp.Data, key, "公开设置不应再包含已裁剪设置键 %s", key)
	}
	// 模型广场（批次 3）：两个公开开关键随广场页一并移除，不许回流。
	for _, key := range []string{
		"model_plaza_enabled",
		"model_plaza_require_auth",
	} {
		require.NotContainsf(t, resp.Data, key, "公开设置不应再包含模型广场键 %s", key)
	}
	// 注册体系遗留键：注册删除后已无消费者，随批次 3 一并清理。
	// 兄弟键 registration_email_suffix_whitelist 是保留面（默认设置种子探测键），
	// 仍应出现在公开设置里。
	require.NotContains(t, resp.Data, "registration_email_domain_quota_enabled")
	require.Contains(t, resp.Data, "registration_email_suffix_whitelist")
}
