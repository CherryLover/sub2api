package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// NotificationErrorDigestHandler 管理端「定时报错汇总」配置：读取 / 保存 / 立即试推。
// 路由挂在 /api/v1/admin/notifications/error-digest，鉴权级别与同组的 Bark 配置一致。
type NotificationErrorDigestHandler struct {
	digestService *service.OpsErrorDigestService
}

func NewNotificationErrorDigestHandler(digestService *service.OpsErrorDigestService) *NotificationErrorDigestHandler {
	return &NotificationErrorDigestHandler{digestService: digestService}
}

// GetErrorDigestConfig 返回当前配置；从未保存过时回默认值。
// GET /api/v1/admin/notifications/error-digest
func (h *NotificationErrorDigestHandler) GetErrorDigestConfig(c *gin.Context) {
	cfg, err := h.digestService.GetErrorDigestConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateErrorDigestConfig 保存配置，保存成功后 cron 立即按新 schedule 重建，不需要重启。
// PUT /api/v1/admin/notifications/error-digest
func (h *NotificationErrorDigestHandler) UpdateErrorDigestConfig(c *gin.Context) {
	var req service.OpsErrorDigestConfigInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.digestService.UpdateErrorDigestConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// TestErrorDigest 立刻按最近 24 小时算一份汇总并推送，方便站长在真到点之前确认通知长什么样。
// 不看 enabled、不看 skip_when_empty；Bark 没启用时返回 pushed=false 而不是报错。
// POST /api/v1/admin/notifications/error-digest/test
func (h *NotificationErrorDigestHandler) TestErrorDigest(c *gin.Context) {
	result, err := h.digestService.RunManualDigest(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
