package admin

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 「账号容量负载」抽屉的窗口 / 条数边界。
const (
	accountRecentRequestsDefaultMinutes = 15
	accountRecentRequestsMaxMinutes     = 1440
	accountRecentRequestsDefaultLimit   = 50
	accountRecentRequestsMaxLimit       = 200
)

// AccountRecentRequestUser 明细行里的最小用户信息，只给 id 与邮箱。
type AccountRecentRequestUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// AccountRecentRequestAPIKey 明细行里的最小 Key 信息，只给 id 与名称，绝不带 key 明文 / 掩码。
type AccountRecentRequestAPIKey struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// AccountRecentRequestItem 账号最近请求明细行。
// 字段名对应 usage_logs 实际列；model 为展示模型（requested_model 回退 model），
// upstream_model 仅在发生了模型映射时出现。
type AccountRecentRequestItem struct {
	ID              int64                       `json:"id"`
	RequestID       string                      `json:"request_id"`
	CreatedAt       time.Time                   `json:"created_at"`
	User            *AccountRecentRequestUser   `json:"user"`
	APIKey          *AccountRecentRequestAPIKey `json:"api_key"`
	Model           string                      `json:"model"`
	UpstreamModel   *string                     `json:"upstream_model,omitempty"`
	RequestType     string                      `json:"request_type"`
	Stream          bool                        `json:"stream"`
	DurationMs      *int                        `json:"duration_ms"`
	FirstTokenMs    *int                        `json:"first_token_ms"`
	InputTokens     int                         `json:"input_tokens"`
	OutputTokens    int                         `json:"output_tokens"`
	CacheReadTokens int                         `json:"cache_read_tokens"`
	TotalCost       float64                     `json:"total_cost"`
	ActualCost      float64                     `json:"actual_cost"`
}

// AccountRecentRequestsResponse GET /admin/accounts/:id/recent-requests 的响应体。
// 不含账号凭据、代理等任何敏感字段。
type AccountRecentRequestsResponse struct {
	AccountID          int64                                `json:"account_id"`
	WindowMinutes      int                                  `json:"window_minutes"`
	CurrentConcurrency int                                  `json:"current_concurrency"`
	MaxConcurrency     int                                  `json:"max_concurrency"`
	WaitingCount       int                                  `json:"waiting_count"`
	TotalRequests      int64                                `json:"total_requests"`
	Items              []AccountRecentRequestItem           `json:"items"`
	ByAPIKey           []usagestats.AccountRecentAPIKeyStat `json:"by_api_key"`
	ByModel            []usagestats.AccountRecentModelStat  `json:"by_model"`
}

// GetRecentRequests 返回账号最近 N 分钟的负载明细：当前并发 / 等待数、整窗请求总数、
// 按 Key 与按模型的聚合，以及最新的 limit 条请求。
// GET /api/v1/admin/accounts/:id/recent-requests?minutes=15&limit=50
func (h *AccountHandler) GetRecentRequests(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	minutes, ok := parseBoundedIntQuery(c, "minutes", accountRecentRequestsDefaultMinutes, 1, accountRecentRequestsMaxMinutes)
	if !ok {
		return
	}
	limit, ok := parseBoundedIntQuery(c, "limit", accountRecentRequestsDefaultLimit, 1, accountRecentRequestsMaxLimit)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	account, err := h.adminService.GetAccount(ctx, accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.accountUsageService == nil {
		response.InternalError(c, "account usage service unavailable")
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(minutes) * time.Minute)
	recent, err := h.accountUsageService.GetAccountRecentRequests(ctx, account.ID, startTime, endTime, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := AccountRecentRequestsResponse{
		AccountID:      account.ID,
		WindowMinutes:  minutes,
		MaxConcurrency: account.Concurrency,
		TotalRequests:  recent.TotalRequests,
		Items:          make([]AccountRecentRequestItem, 0, len(recent.Items)),
		ByAPIKey:       recent.ByAPIKey,
		ByModel:        recent.ByModel,
	}
	if resp.ByAPIKey == nil {
		resp.ByAPIKey = []usagestats.AccountRecentAPIKeyStat{}
	}
	if resp.ByModel == nil {
		resp.ByModel = []usagestats.AccountRecentModelStat{}
	}
	for i := range recent.Items {
		resp.Items = append(resp.Items, accountRecentRequestItemFromService(&recent.Items[i]))
	}

	// 并发 / 等待数复用账号列表同一条读取链路（Redis ZCARD / 等待计数器）；读失败按 0 展示，不阻断明细。
	if h.concurrencyService != nil {
		if counts, err := h.concurrencyService.GetAccountConcurrencyBatch(ctx, []int64{account.ID}); err == nil {
			resp.CurrentConcurrency = counts[account.ID]
		}
		if waiting, err := h.concurrencyService.GetAccountWaitingCount(ctx, account.ID); err == nil {
			resp.WaitingCount = waiting
		}
	}

	response.Success(c, resp)
}

func accountRecentRequestItemFromService(l *service.UsageLog) AccountRecentRequestItem {
	requestType := l.EffectiveRequestType()
	stream, _ := service.ApplyLegacyRequestFields(requestType, l.Stream, l.OpenAIWSMode)
	model := strings.TrimSpace(l.RequestedModel)
	if model == "" {
		model = l.Model
	}

	item := AccountRecentRequestItem{
		ID:              l.ID,
		RequestID:       l.RequestID,
		CreatedAt:       l.CreatedAt.UTC(),
		User:            &AccountRecentRequestUser{ID: l.UserID},
		APIKey:          &AccountRecentRequestAPIKey{ID: l.APIKeyID},
		Model:           model,
		UpstreamModel:   l.UpstreamModel,
		RequestType:     requestType.String(),
		Stream:          stream,
		DurationMs:      l.DurationMs,
		FirstTokenMs:    l.FirstTokenMs,
		InputTokens:     l.InputTokens,
		OutputTokens:    l.OutputTokens,
		CacheReadTokens: l.CacheReadTokens,
		TotalCost:       l.TotalCost,
		ActualCost:      l.ActualCost,
	}
	if l.User != nil {
		item.User.Email = l.User.Email
	}
	if l.APIKey != nil {
		item.APIKey.Name = l.APIKey.Name
	}
	return item
}

// parseBoundedIntQuery 读取整数 query 参数：缺省用 def，非整数或越界直接 400 并返回 false。
func parseBoundedIntQuery(c *gin.Context, name string, def, minValue, maxValue int) (int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return def, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > maxValue {
		response.BadRequest(c, fmt.Sprintf("%s must be an integer between %d and %d", name, minValue, maxValue))
		return 0, false
	}
	return value, true
}
