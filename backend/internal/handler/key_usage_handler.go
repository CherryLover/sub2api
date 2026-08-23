package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	ippkg "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// keyUsageGenericAuthMessage 是免登录用量页唯一的鉴权失败文案。
// key 不存在、key 被禁用、令牌过期/被篡改共用同一句话与同一个状态码，
// 否则这个端点就成了"批量探测某个 key 是否存在"的接口。
const keyUsageGenericAuthMessage = "Invalid or expired key"

// KeyUsageHandler 免登录用量页（/key-usage）的公开 API。
type KeyUsageHandler struct {
	keyUsageService     *service.KeyUsageService
	gateway             *GatewayHandler
	subscriptionService *service.SubscriptionService
}

// NewKeyUsageHandler 创建 handler。gateway 用于复用 /v1/usage 的 payload 组装逻辑。
func NewKeyUsageHandler(keyUsageService *service.KeyUsageService, gateway *GatewayHandler, subscriptionService *service.SubscriptionService) *KeyUsageHandler {
	return &KeyUsageHandler{
		keyUsageService:     keyUsageService,
		gateway:             gateway,
		subscriptionService: subscriptionService,
	}
}

// KeyUsageSessionRequest 是 key → 令牌交换的请求体。
type KeyUsageSessionRequest struct {
	Key string `json:"key"`
}

// KeyUsageSessionResponse 是 key → 令牌交换的响应体。
type KeyUsageSessionResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// KeyUsageReportResponse 是用量报告的响应体。
//
// usage 复用 /v1/usage 的组装逻辑（同一个 buildUsagePayload），但不是逐字节等价：
// 免登录路径没有 API Key 中间件，订阅数据由本 handler 自行查询，因此在 simple 运行模式下
// 这里会比 /v1/usage 多出 subscription 对象（simple 模式的中间件在设置 context 前就返回了，
// /v1/usage 拿不到订阅）。其余模式下两者字段一致。
//
// usage_available 区分"后端组装用量失败"（usage = null，false）与"确实没有数据"
// （usage 是对象但内容为空，true）；没有它前端只能看到一个空对象，无法提示用户重试。
// windows / rankings 恒为对象，无数据时是零值。
type KeyUsageReportResponse struct {
	Key            service.KeyUsageKeyInfo  `json:"key"`
	Usage          any                      `json:"usage"`
	UsageAvailable bool                     `json:"usage_available"`
	Windows        service.KeyUsageWindows  `json:"windows"`
	Rankings       service.KeyUsageRankings `json:"rankings"`
	Metric         string                   `json:"metric"`
	GeneratedAt    time.Time                `json:"generated_at"`
}

// CreateSession 用 API Key 换取只读用量令牌。
// POST /api/v1/key-usage/session
//
// 令牌只能用来看这把 key 的用量，网关不接受它作为凭证；原始 key 不进 URL，
// 避免通过浏览器历史 / Referer / 访问日志外泄。
func (h *KeyUsageHandler) CreateSession(c *gin.Context) {
	if h.keyUsageService == nil {
		keyUsageErrorResponse(c, http.StatusServiceUnavailable, "Key usage lookup is unavailable")
		return
	}

	var req KeyUsageSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		keyUsageErrorResponse(c, http.StatusUnauthorized, keyUsageGenericAuthMessage)
		return
	}

	token, expiresAt, err := h.keyUsageService.IssueToken(c.Request.Context(), req.Key, keyUsageClientIP(c))
	if err != nil {
		// 签名密钥缺失属于服务端配置问题，与 key 是否有效无关，单独用 503 暴露出来。
		if infraerrors.IsServiceUnavailable(err) {
			keyUsageErrorResponse(c, http.StatusServiceUnavailable, "Key usage tokens are not configured")
			return
		}
		keyUsageErrorResponse(c, http.StatusUnauthorized, keyUsageGenericAuthMessage)
		return
	}

	c.JSON(http.StatusOK, KeyUsageSessionResponse{Token: token, ExpiresAt: expiresAt})
}

// Report 返回用量 + 排名报告。
// GET /api/v1/key-usage/report?token=<token>&metric=<cost|tokens|requests>
//
// 两条鉴权路径二选一：带 token 参数走令牌；否则读 Authorization: Bearer <key>
// （首次输入 key 时可以一次拿到数据，不必先换令牌）。两条路的响应体完全一致。
func (h *KeyUsageHandler) Report(c *gin.Context) {
	if h.keyUsageService == nil {
		keyUsageErrorResponse(c, http.StatusServiceUnavailable, "Key usage lookup is unavailable")
		return
	}

	ctx := c.Request.Context()

	var (
		apiKey *service.APIKey
		err    error
	)
	clientIP := keyUsageClientIP(c)
	if token := strings.TrimSpace(c.Query("token")); token != "" {
		apiKey, err = h.keyUsageService.ResolveToken(ctx, token, clientIP)
	} else if rawKey := keyUsageRawKeyFromRequest(c); rawKey != "" {
		apiKey, err = h.keyUsageService.ResolveRawKey(ctx, rawKey, clientIP)
	} else {
		keyUsageErrorResponse(c, http.StatusUnauthorized, keyUsageGenericAuthMessage)
		return
	}
	if err != nil || apiKey == nil {
		keyUsageErrorResponse(c, http.StatusUnauthorized, keyUsageGenericAuthMessage)
		return
	}

	// 窗口边界跟随前端传来的浏览器时区，与 /v1/usage 的按天曲线（timezone query param）同源。
	report := h.keyUsageService.BuildReport(ctx, apiKey, c.Query("metric"), c.Query("timezone"))

	// usage 复用 /v1/usage 的组装逻辑。失败时下发 null + usage_available=false，
	// 而不是空对象：空对象与"这把 key 真的没有用量"在前端无法区分，
	// 后端挂了会被静默渲染成"一切正常，只是没数据"。
	var (
		usagePayload   any
		usageAvailable bool
	)
	if h.gateway != nil {
		payload, buildErr := h.gateway.BuildAPIKeyUsagePayload(c, apiKey, h.resolveSubscription(c, apiKey))
		if buildErr != nil {
			slog.Warn("key usage payload build failed", "api_key_id", apiKey.ID, "error", buildErr)
		} else if payload != nil {
			usagePayload = payload
			usageAvailable = true
		}
	}

	c.JSON(http.StatusOK, KeyUsageReportResponse{
		Key:            report.Key,
		Usage:          usagePayload,
		UsageAvailable: usageAvailable,
		Windows:        report.Windows,
		Rankings:       report.Rankings,
		Metric:         report.Metric,
		GeneratedAt:    timezone.Now(),
	})
}

// keyUsageClientIP 返回用于 API Key IP 白名单/黑名单校验的客户端地址。
//
// 与网关 API Key 中间件同源：中间件调用 ip.GetSecurityClientIP(c, cfg.TrustForwardedIPForAPIKeyACL())，
// 而全局中间件 SessionBindingContext 已经把同一个开关按请求快照进了 context，
// GetSecurityClientIP 会优先读该快照，因此这里传 false 得到的结果与网关完全一致；
// 快照缺失（中间件未挂载）时 false 会回落到 server.trusted_proxies 可信链，是更保守的一侧。
func keyUsageClientIP(c *gin.Context) string {
	return ippkg.GetSecurityClientIP(c, false)
}

// resolveSubscription 免登录路径没有经过 API Key 中间件，订阅数据要自己查。
// 尽力而为：查不到就按"无订阅"渲染，不影响其余板块。
func (h *KeyUsageHandler) resolveSubscription(c *gin.Context, apiKey *service.APIKey) *service.UserSubscription {
	if h.subscriptionService == nil || apiKey.Group == nil || !apiKey.Group.IsSubscriptionType() {
		return nil
	}
	subscription, err := h.subscriptionService.GetActiveSubscription(c.Request.Context(), apiKey.UserID, apiKey.Group.ID)
	if err != nil {
		return nil
	}
	return subscription
}

// keyUsageRawKeyFromRequest 从请求头提取原始 API Key（Bearer 优先，兼容 x-api-key）。
func keyUsageRawKeyFromRequest(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization != "" {
		if len(authorization) > 7 && strings.EqualFold(authorization[:7], "bearer ") {
			return strings.TrimSpace(authorization[7:])
		}
		return authorization
	}
	return strings.TrimSpace(c.GetHeader("x-api-key"))
}

func keyUsageErrorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": gin.H{"message": message}})
}
