package admin

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// opsRequestChainMaxIDLen 与 ops_error_logs.client_request_id 的 VARCHAR(64) 对齐后
// 再留一倍余量：超长的路径参数直接挡掉，不进服务层、不进 SQL。
const opsRequestChainMaxIDLen = 128

// GetRequestChain 返回一次客户端请求的完整上游链路：网关试了几次、每次撞在哪个账号上、
// 最终是被重试/换号兜住了（recovered）还是真的失败了（failed）。
//
// 账号列表上的「上游错误」只会显示「502 × 5」，站长看到会以为出了五次事故；这个接口
// 就是用来解释「后续发生了什么」的。判定口径见 service/ops_account_recent_errors.go 顶部。
//
// GET /api/v1/admin/ops/requests/:clientRequestId/chain
func (h *OpsHandler) GetRequestChain(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	clientRequestID := strings.TrimSpace(c.Param("clientRequestId"))
	if clientRequestID == "" || len(clientRequestID) > opsRequestChainMaxIDLen {
		response.BadRequest(c, "Invalid client request id")
		return
	}

	chain, err := h.opsService.GetRequestChain(c.Request.Context(), clientRequestID)
	if err != nil {
		// 找不到该请求走 ErrOpsRequestChainNotFound，ErrorFrom 会映射成 404。
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, chain)
}
