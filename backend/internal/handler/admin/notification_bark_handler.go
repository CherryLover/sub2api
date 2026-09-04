package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// NotificationBarkHandler 管理端 Bark 推送通道配置：读取 / 保存 / 测试推送。
// 路由挂在 /api/v1/admin/notifications/bark，鉴权级别与 /admin/backups/s3-config 相同。
type NotificationBarkHandler struct {
	barkService *service.BarkNotificationService
}

func NewNotificationBarkHandler(barkService *service.BarkNotificationService) *NotificationBarkHandler {
	return &NotificationBarkHandler{barkService: barkService}
}

// GetBarkConfig 返回脱敏配置（device_key 永远为空串，has_device_key 表示已配置）。
// GET /api/v1/admin/notifications/bark
func (h *NotificationBarkHandler) GetBarkConfig(c *gin.Context) {
	cfg, err := h.barkService.GetBarkConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateBarkConfig 保存配置：device_key 为空则保留已存值，非空则加密覆盖。
// PUT /api/v1/admin/notifications/bark
func (h *NotificationBarkHandler) UpdateBarkConfig(c *gin.Context) {
	var req service.BarkConfigInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.barkService.UpdateBarkConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// TestBark 用请求体里的配置直接发一条测试通知（device_key 为空时用已存的）。
// POST /api/v1/admin/notifications/bark/test
func (h *NotificationBarkHandler) TestBark(c *gin.Context) {
	var req service.BarkTestInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.barkService.TestBark(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
