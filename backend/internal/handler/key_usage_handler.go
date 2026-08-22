package handler

import (
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
// usage 原样内嵌 /v1/usage 的 payload；windows / rankings 恒为对象，无数据时是零值。
type KeyUsageReportResponse struct {
	Key         service.KeyUsageKeyInfo  `json:"key"`
	Usage       any                      `json:"usage"`
	Windows     service.KeyUsageWindows  `json:"windows"`
	Rankings    service.KeyUsageRankings `json:"rankings"`
	Metric      string                   `json:"metric"`
	GeneratedAt time.Time                `json:"generated_at"`
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

	token, expiresAt, err := h.keyUsageService.IssueToken(c.Request.Context(), req.Key)
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
	if token := strings.TrimSpace(c.Query("token")); token != "" {
		apiKey, err = h.keyUsageService.ResolveToken(ctx, token)
	} else if rawKey := keyUsageRawKeyFromRequest(c); rawKey != "" {
		apiKey, err = h.keyUsageService.ResolveRawKey(ctx, rawKey)
	} else {
		keyUsageErrorResponse(c, http.StatusUnauthorized, keyUsageGenericAuthMessage)
		return
	}
	if err != nil || apiKey == nil {
		keyUsageErrorResponse(c, http.StatusUnauthorized, keyUsageGenericAuthMessage)
		return
	}

	report := h.keyUsageService.BuildReport(ctx, apiKey, c.Query("metric"))

	// usage 复用 /v1/usage 的组装逻辑；失败时退化成空对象而不是 null，前端渲染路径唯一。
	var usagePayload any = gin.H{}
	if h.gateway != nil {
		if payload, buildErr := h.gateway.BuildAPIKeyUsagePayload(c, apiKey, h.resolveSubscription(c, apiKey)); buildErr == nil && payload != nil {
			usagePayload = payload
		}
	}

	c.JSON(http.StatusOK, KeyUsageReportResponse{
		Key:         report.Key,
		Usage:       usagePayload,
		Windows:     report.Windows,
		Rankings:    report.Rankings,
		Metric:      report.Metric,
		GeneratedAt: timezone.Now(),
	})
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
