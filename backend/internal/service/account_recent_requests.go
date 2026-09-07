package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// AccountRecentRequestBreakdownReader 是 UsageLogRepository 的可选扩展：
// 按账号 + 时间窗口做整窗 GROUP BY 聚合（总数 / 按 Key / 按模型），不受明细条数截断。
// 与 accountWindowStatsBatchReader 同一套路：运行时类型断言，避免把大接口继续撑大。
type AccountRecentRequestBreakdownReader interface {
	GetAccountRecentRequestBreakdown(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountRecentRequestBreakdown, error)
}

// ErrAccountRecentRequestsUnsupported 当前 usage log 仓储不支持整窗聚合时返回。
var ErrAccountRecentRequestsUnsupported = errors.New("usage log repository does not support account recent request breakdown")

// AccountRecentRequests 账号最近窗口的负载明细：整窗聚合 + 最新的若干条明细。
type AccountRecentRequests struct {
	StartTime     time.Time
	EndTime       time.Time
	TotalRequests int64
	// Items 按 created_at 倒序，最多 limit 条；关联的 User / APIKey 已批量补齐。
	Items    []UsageLog
	ByAPIKey []usagestats.AccountRecentAPIKeyStat
	ByModel  []usagestats.AccountRecentModelStat
}

// GetAccountRecentRequests 取账号在 [startTime, endTime) 内的整窗聚合与最新 limit 条明细。
// limit <= 0 时只做聚合、不拉明细。
func (s *AccountUsageService) GetAccountRecentRequests(ctx context.Context, accountID int64, startTime, endTime time.Time, limit int) (*AccountRecentRequests, error) {
	if s == nil || s.usageLogRepo == nil {
		return nil, ErrAccountRecentRequestsUnsupported
	}
	reader, ok := s.usageLogRepo.(AccountRecentRequestBreakdownReader)
	if !ok {
		return nil, ErrAccountRecentRequestsUnsupported
	}

	breakdown, err := reader.GetAccountRecentRequestBreakdown(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get account recent request breakdown failed: %w", err)
	}

	result := &AccountRecentRequests{
		StartTime:     startTime,
		EndTime:       endTime,
		TotalRequests: breakdown.TotalRequests,
		Items:         []UsageLog{},
		ByAPIKey:      breakdown.ByAPIKey,
		ByModel:       breakdown.ByModel,
	}
	if result.ByAPIKey == nil {
		result.ByAPIKey = []usagestats.AccountRecentAPIKeyStat{}
	}
	if result.ByModel == nil {
		result.ByModel = []usagestats.AccountRecentModelStat{}
	}

	// 空窗口直接返回，省掉一次明细查询；limit <= 0 表示调用方只要聚合。
	if breakdown.TotalRequests == 0 || limit <= 0 {
		return result, nil
	}

	params := pagination.PaginationParams{
		Page:      1,
		PageSize:  limit,
		SortBy:    "created_at",
		SortOrder: pagination.SortOrderDesc,
	}
	filters := usagestats.UsageLogFilters{
		AccountID: accountID,
		StartTime: &startTime,
		EndTime:   &endTime,
	}
	items, _, err := s.usageLogRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, fmt.Errorf("list account recent requests failed: %w", err)
	}
	if items != nil {
		result.Items = items
	}
	return result, nil
}
