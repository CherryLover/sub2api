package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// accountRecentCostExpr 账号口径费用，与账号列表「今日费用」(GetAccountTodayStats) 使用同一公式，
// 保证账号页各处看到的金额口径一致。
const accountRecentCostExpr = "COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0)"

// GetAccountRecentRequestBreakdown 按账号 + 时间窗口做整窗聚合，供「账号容量负载」抽屉使用。
//
// 设计取舍：
//   - 两条 GROUP BY SQL 走完，WHERE 只带 account_id + created_at 范围，命中
//     idx_usage_logs_account_created_at；不按明细逐条统计，所以不受明细条数上限截断。
//   - by_api_key 直接 LEFT JOIN api_keys 取名称：单账号短窗口的候选 Key 数很小，
//     JOIN 比再发一次批量查名便宜；软删除的 Key 仍返回名称，历史请求要能解释来源。
//   - by_model 按展示模型聚合（requested_model 为空回退 model），与明细里的 model 字段同口径。
//   - total_requests 由 by_api_key 的 count 求和得到，与两组聚合天然一致。
func (r *usageLogRepository) GetAccountRecentRequestBreakdown(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountRecentRequestBreakdown, error) {
	byAPIKey, err := r.queryAccountRecentAPIKeyStats(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	byModel, err := r.queryAccountRecentModelStats(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	result := &usagestats.AccountRecentRequestBreakdown{
		ByAPIKey: byAPIKey,
		ByModel:  byModel,
	}
	for _, row := range byAPIKey {
		result.TotalRequests += row.Count
	}
	return result, nil
}

func (r *usageLogRepository) queryAccountRecentAPIKeyStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (results []usagestats.AccountRecentAPIKeyStat, err error) {
	query := `
		SELECT
			ul.api_key_id,
			COALESCE(k.name, '') AS key_name,
			COUNT(*) AS requests,
			` + accountRecentCostExpr + ` AS cost
		FROM usage_logs ul
		LEFT JOIN api_keys k ON k.id = ul.api_key_id
		WHERE ul.account_id = $1 AND ul.created_at >= $2 AND ul.created_at < $3
		GROUP BY ul.api_key_id, k.name
		ORDER BY COUNT(*) DESC, ul.api_key_id ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		// 保持主错误优先；仅在无错误时回传 Close 失败，并清空不完整结果。
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.AccountRecentAPIKeyStat, 0, 8)
	for rows.Next() {
		var row usagestats.AccountRecentAPIKeyStat
		if scanErr := rows.Scan(&row.APIKeyID, &row.Name, &row.Count, &row.Cost); scanErr != nil {
			return nil, scanErr
		}
		results = append(results, row)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return results, nil
}

func (r *usageLogRepository) queryAccountRecentModelStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (results []usagestats.AccountRecentModelStat, err error) {
	query := `
		SELECT
			COALESCE(NULLIF(TRIM(ul.requested_model), ''), ul.model) AS display_model,
			COUNT(*) AS requests,
			` + accountRecentCostExpr + ` AS cost
		FROM usage_logs ul
		WHERE ul.account_id = $1 AND ul.created_at >= $2 AND ul.created_at < $3
		GROUP BY COALESCE(NULLIF(TRIM(ul.requested_model), ''), ul.model)
		ORDER BY COUNT(*) DESC, display_model ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.AccountRecentModelStat, 0, 8)
	for rows.Next() {
		var row usagestats.AccountRecentModelStat
		if scanErr := rows.Scan(&row.Model, &row.Count, &row.Cost); scanErr != nil {
			return nil, scanErr
		}
		results = append(results, row)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return results, nil
}
