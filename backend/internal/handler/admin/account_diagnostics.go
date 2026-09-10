package admin

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	accountDiagnosticsDefaultWindowMinutes = 15
	accountDiagnosticsMinWindowMinutes     = 5
	accountDiagnosticsMaxWindowMinutes     = 1440
)

// BatchAccountDiagnosticsRequest 批量账号诊断请求体。
type BatchAccountDiagnosticsRequest struct {
	AccountIDs    []int64 `json:"account_ids" binding:"required"`
	WindowMinutes int     `json:"window_minutes"`
}

// AccountDiagnostics 单个账号的诊断结果。
// RecentErrors 为 nil 表示错误数据不可用（运维监控关闭或查询失败），不等于"没有错误"。
type AccountDiagnostics struct {
	Scheduling   service.AccountSchedulingDiagnosis    `json:"scheduling"`
	RecentErrors *service.OpsAccountRecentErrorSummary `json:"recent_errors"`
}

// BatchAccountDiagnosticsResponse 批量账号诊断响应体，diagnostics 以账号 ID 字符串为键。
type BatchAccountDiagnosticsResponse struct {
	WindowMinutes int                           `json:"window_minutes"`
	GeneratedAt   time.Time                     `json:"generated_at"`
	Diagnostics   map[string]AccountDiagnostics `json:"diagnostics"`
}

func normalizeAccountDiagnosticsWindow(minutes int) int {
	switch {
	case minutes <= 0:
		return accountDiagnosticsDefaultWindowMinutes
	case minutes < accountDiagnosticsMinWindowMinutes:
		return accountDiagnosticsMinWindowMinutes
	case minutes > accountDiagnosticsMaxWindowMinutes:
		return accountDiagnosticsMaxWindowMinutes
	default:
		return minutes
	}
}

// GetBatchDiagnostics 批量诊断账号当前能否被调度（含网关进程内的熔断/冷却）以及近期上游错误。
// 进程内状态按实例独立，结果只反映处理本次请求的实例。实时状态，不走快照缓存。
// POST /api/v1/admin/accounts/diagnostics/batch
func (h *AccountHandler) GetBatchDiagnostics(c *gin.Context) {
	var req BatchAccountDiagnosticsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	windowMinutes := normalizeAccountDiagnosticsWindow(req.WindowMinutes)
	now := time.Now().UTC()
	payload := BatchAccountDiagnosticsResponse{
		WindowMinutes: windowMinutes,
		GeneratedAt:   now,
		Diagnostics:   map[string]AccountDiagnostics{},
	}

	accountIDs := normalizeInt64IDList(req.AccountIDs)
	if len(accountIDs) == 0 {
		response.Success(c, payload)
		return
	}

	ctx := c.Request.Context()
	accounts, err := h.adminService.GetAccountsByIDs(ctx, accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	existingIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			existingIDs = append(existingIDs, account.ID)
		}
	}

	// nil gateway is fine: only persisted blocks are reported then.
	scheduling := h.openAIGatewayService.DiagnoseAccountScheduling(ctx, accounts)

	var recentErrors map[int64]*service.OpsAccountRecentErrorSummary
	if h.opsService != nil && len(existingIDs) > 0 {
		since := now.Add(-time.Duration(windowMinutes) * time.Minute)
		recentErrors, err = h.opsService.GetAccountRecentErrorSummaries(ctx, existingIDs, since)
		if err != nil {
			slog.Warn("account_diagnostics_recent_errors_failed",
				"account_count", len(existingIDs),
				"window_minutes", windowMinutes,
				"error", err)
			recentErrors = nil
		}
	}

	for _, account := range accounts {
		if account == nil {
			continue
		}
		entry := AccountDiagnostics{Scheduling: scheduling[account.ID]}
		if recentErrors != nil {
			entry.RecentErrors = recentErrors[account.ID]
		}
		payload.Diagnostics[strconv.FormatInt(account.ID, 10)] = entry
	}
	response.Success(c, payload)
}
