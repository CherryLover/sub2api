package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"

	"github.com/lib/pq"
)

// keyUsageRankingOrderColumns 把外部传入的排序指标映射为 SELECT 里的列别名。
// 这里必须走白名单：metric 来自免登录端点的 query param，直接拼进 ORDER BY 就是注入面。
var keyUsageRankingOrderColumns = map[string]string{
	usagestats.KeyRankingMetricCost:     "cost",
	usagestats.KeyRankingMetricTokens:   "tokens",
	usagestats.KeyRankingMetricRequests: "requests",
}

// GetAPIKeyUsageAggregates 按 api_key_id 聚合某个时间窗口内的用量，用于 Key 排行榜。
//
// 设计取舍：
//   - 一条 SQL 走完（GROUP BY api_key_id），绝不按 Key 循环查询；名次、前十名、并列
//     判定全部放到调用方内存里做。
//   - 不 JOIN api_keys：全站榜候选行可能上万，只有前十名需要展示名称，名称单独用
//     GetAPIKeyNamesByIDs 按主键批量取回，避免把整张榜的名称拉过网络。
//   - userID > 0 时走 (user_id, created_at) 复合索引（账户内榜）；userID == 0 时按
//     created_at 范围扫描（全站榜），调用方必须对结果做缓存。
//   - cost 口径用 actual_cost（实际扣费），与页面上 /v1/usage 展示的 actual_cost 一致。
func (r *usageLogRepository) GetAPIKeyUsageAggregates(ctx context.Context, startTime, endTime time.Time, userID int64, metric string) (results []usagestats.APIKeyUsageAggregate, err error) {
	orderColumn, ok := keyUsageRankingOrderColumns[metric]
	if !ok {
		return nil, fmt.Errorf("unsupported key ranking metric: %q", metric)
	}

	query := `
		SELECT
			api_key_id,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS tokens,
			COALESCE(SUM(actual_cost), 0) AS cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`
	args := []any{startTime, endTime}
	if userID > 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
	}
	// api_key_id 作为次级排序键，保证并列指标下的展示顺序稳定（否则同一份数据两次请求顺序可能不同）。
	query += fmt.Sprintf(" GROUP BY api_key_id ORDER BY %s DESC, api_key_id ASC", orderColumn)

	rows, err := r.sql.QueryContext(ctx, query, args...)
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

	results = make([]usagestats.APIKeyUsageAggregate, 0, 64)
	for rows.Next() {
		var row usagestats.APIKeyUsageAggregate
		if scanErr := rows.Scan(&row.APIKeyID, &row.Requests, &row.Tokens, &row.Cost); scanErr != nil {
			return nil, scanErr
		}
		results = append(results, row)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return results, nil
}

// GetAPIKeyNamesByIDs 按主键批量取回 Key 名称，供排行榜展示。
// 只查前若干名需要的 ID，走 api_keys 主键索引；软删除的 Key 依然返回名称，
// 因为它历史上确实产生过用量，榜单需要能解释这行数据的来源。
func (r *usageLogRepository) GetAPIKeyNamesByIDs(ctx context.Context, ids []int64) (names map[int64]string, err error) {
	names = make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}

	var rows *sql.Rows
	rows, err = r.sql.QueryContext(ctx, `SELECT id, COALESCE(name, '') FROM api_keys WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			names = nil
		}
	}()

	for rows.Next() {
		var (
			id   int64
			name string
		)
		if scanErr := rows.Scan(&id, &name); scanErr != nil {
			return nil, scanErr
		}
		names[id] = name
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return names, nil
}
