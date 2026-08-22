package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterKeyUsageRoutes 注册免登录用量页（/key-usage）的公开路由。
//
// 这是无鉴权分组：POST /session 接受 API Key 明文输入，天然是"批量验证 key 是否有效"
// 的探测面，因此按客户端 IP 严格限流且 Redis 故障时 fail-close（宁可拒绝也不放行探测）；
// GET /report 只读且带签名令牌，限流放宽并 fail-open，避免 Redis 抖动把页面打挂。
// 两个端点再叠加一层面板公开接口的 IP 限流（阈值可在系统设置里调）。
func RegisterKeyUsageRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	redisClient *redis.Client,
	panelRateLimiter *servermiddleware.PanelRateLimiter,
) {
	if h == nil || h.KeyUsage == nil {
		return
	}

	rateLimiter := middleware.NewRateLimiter(redisClient)

	keyUsage := v1.Group("/key-usage")
	keyUsage.Use(panelRateLimiter.PublicIP())
	{
		keyUsage.POST("/session",
			rateLimiter.LimitWithOptions("key-usage-session", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.KeyUsage.CreateSession,
		)
		keyUsage.GET("/report",
			rateLimiter.LimitWithOptions("key-usage-report", 60, time.Minute, middleware.RateLimitOptions{}),
			h.KeyUsage.Report,
		)
	}
}
