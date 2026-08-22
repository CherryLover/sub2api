package routes

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// keyUsageProbeIPRPM 是"提交原始凭证"路径上的 per-IP 阈值（每分钟）。
// POST /session 与 GET /report + Bearer 是完全等效的两个 oracle，必须同阈值、同故障策略。
const keyUsageProbeIPRPM = 10

// keyUsageTokenIPRPM 是令牌路径（GET /report?token=）的 per-IP 阈值（每分钟）。
// 令牌猜不出来，不构成探测面，阈值放宽以容忍页面自动刷新。
const keyUsageTokenIPRPM = 60

// RegisterKeyUsageRoutes 注册免登录用量页（/key-usage）的公开路由。
//
// 这是无鉴权分组，且 POST /session 与 GET /report + Bearer 都接受 API Key 明文输入，
// 天然是"批量验证 key 是否有效"的探测面。防护分三层，缺一不可：
//
//  1. KeyUsageAbuseGuard：与客户端 IP 完全无关的端点级全局预算 + 网关的无效鉴权封禁计数。
//     这是唯一挡得住 X-Forwarded-For 轮换的一层（见 key_usage_guard.go 里的说明）。
//  2. per-IP 固定窗口限流：对普通滥用仍然有效。凭证路径严格且 Redis 故障时 fail-close
//     （宁可拒绝也不放行探测）；令牌路径放宽且 fail-open，避免 Redis 抖动把页面打挂。
//  3. 面板公开接口的 IP 限流（阈值可在系统设置里调）。
//
// cfg.KeyUsage.Enabled 为 false 时整组路由不注册：这是出事时不用改代码发版就能关掉
// 这个"公开全站 Key 名称与用量"页面的开关，前端已按 404 处理未部署的端点。
func RegisterKeyUsageRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	redisClient *redis.Client,
	panelRateLimiter *servermiddleware.PanelRateLimiter,
	apiKeyService *service.APIKeyService,
	cfg *config.Config,
) {
	if h == nil || h.KeyUsage == nil {
		return
	}
	if cfg == nil || !cfg.KeyUsage.Enabled {
		return
	}

	rateLimiter := middleware.NewRateLimiter(redisClient)
	guard := servermiddleware.NewKeyUsageAbuseGuard(apiKeyService)

	probeIPLimit := rateLimiter.LimitWithOptions("key-usage-probe", keyUsageProbeIPRPM, time.Minute, middleware.RateLimitOptions{
		FailureMode: middleware.RateLimitFailClose,
	})
	tokenIPLimit := rateLimiter.LimitWithOptions("key-usage-report-token", keyUsageTokenIPRPM, time.Minute, middleware.RateLimitOptions{})

	keyUsage := v1.Group("/key-usage")
	keyUsage.Use(panelRateLimiter.PublicIP())
	{
		keyUsage.POST("/session", guard.Handler(), probeIPLimit, h.KeyUsage.CreateSession)
		keyUsage.GET("/report", guard.Handler(), keyUsageReportIPLimit(probeIPLimit, tokenIPLimit), h.KeyUsage.Report)
	}
}

// keyUsageReportIPLimit 按凭证类型选择 /report 的 per-IP 限流档位。
// 带 token 的请求走宽松 fail-open 档，其余（Bearer 原始 key / 无凭证）与 /session 同档。
func keyUsageReportIPLimit(probe, tokenPath gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.Query("token")) != "" {
			tokenPath(c)
			return
		}
		probe(c)
	}
}
