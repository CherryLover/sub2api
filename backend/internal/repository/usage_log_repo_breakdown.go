package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// breakdownSortColumns 是排行的合法排序列 allowlist（非用户原样字符串），避免 SQL 注入。
var breakdownSortColumns = map[string]string{
	"requests":      "requests",
	"input_tokens":  "input_tokens",
	"output_tokens": "output_tokens",
	"cache_tokens":  "cache_tokens",
	"total_tokens":  "total_tokens",
	"cost":          "cost",
	"actual_cost":   "actual_cost",
}

// breakdownOrderColumn 返回排序列；空值或非法值回退到 actual_cost。
func breakdownOrderColumn(sortBy string) string {
	if col, ok := breakdownSortColumns[sortBy]; ok {
		return col
	}
	return "actual_cost"
}

// breakdownFilterClause 把用户排行与 API Key 排行共用的筛选条件拼成 SQL 片段。
// 片段里每条条件都以 " AND " 开头，调用方负责先写 created_at 的 WHERE。
func breakdownFilterClause(dim usagestats.UserBreakdownDimension, args []any) (string, []any) {
	clause := ""
	if dim.GroupID > 0 {
		clause += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, dim.GroupID)
	}
	if dim.Model != "" {
		clause += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpression(dim.ModelType), len(args)+1)
		args = append(args, dim.Model)
	}
	if dim.Endpoint != "" {
		col := resolveEndpointColumn(dim.EndpointType)
		clause += fmt.Sprintf(" AND %s = $%d", col, len(args)+1)
		args = append(args, dim.Endpoint)
	}
	if dim.UserID > 0 {
		clause += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, dim.UserID)
	}
	if dim.APIKeyID > 0 {
		clause += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, dim.APIKeyID)
	}
	if dim.AccountID > 0 {
		clause += fmt.Sprintf(" AND ul.account_id = $%d", len(args)+1)
		args = append(args, dim.AccountID)
	}
	if dim.RequestType != nil {
		condition, conditionArgs := buildRequestTypeFilterConditionWithAlias(len(args)+1, *dim.RequestType, "ul")
		clause += " AND " + condition
		args = append(args, conditionArgs...)
	}
	if dim.Stream != nil {
		clause += fmt.Sprintf(" AND ul.stream = $%d", len(args)+1)
		args = append(args, *dim.Stream)
	}
	if dim.BillingType != nil {
		clause += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, *dim.BillingType)
	}
	return clause, args
}

// GetAPIKeyBreakdownStats returns per-API-key usage breakdown within a dimension.
// 与 GetUserBreakdownStats 同口径，只是聚合维度换成 api_key_id，并带出 Key 名与归属用户。
func (r *usageLogRepository) GetAPIKeyBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim usagestats.UserBreakdownDimension, limit int) (results []usagestats.APIKeyBreakdownItem, err error) {
	query := `
		SELECT
			COALESCE(ul.api_key_id, 0) as api_key_id,
			COALESCE(k.name, '') as name,
			COALESCE(ul.user_id, 0) as user_id,
			COALESCE(u.email, '') as email,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens), 0) as input_tokens,
			COALESCE(SUM(ul.output_tokens), 0) as output_tokens,
			COALESCE(SUM(ul.cache_creation_tokens + ul.cache_read_tokens), 0) as cache_tokens,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost
		FROM usage_logs ul
		LEFT JOIN api_keys k ON k.id = ul.api_key_id
		LEFT JOIN users u ON u.id = ul.user_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`
	args := []any{startTime, endTime}
	clause, args := breakdownFilterClause(dim, args)
	query += clause
	query += " GROUP BY ul.api_key_id, k.name, ul.user_id, u.email"
	query += " ORDER BY " + breakdownOrderColumn(dim.SortBy) + " DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.APIKeyBreakdownItem, 0)
	for rows.Next() {
		var row usagestats.APIKeyBreakdownItem
		if err := rows.Scan(
			&row.APIKeyID,
			&row.Name,
			&row.UserID,
			&row.Email,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheTokens,
			&row.TotalTokens,
			&row.Cost,
			&row.ActualCost,
		); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// breakdownModelColumn 是内部 allowlist，杜绝把列名拼进 SQL 的注入面。
func breakdownModelColumn(dim usagestats.BreakdownModelDimension) (string, bool) {
	switch dim {
	case usagestats.BreakdownModelByUser:
		return "ul.user_id", true
	case usagestats.BreakdownModelByAPIKey:
		return "ul.api_key_id", true
	default:
		return "", false
	}
}

// GetBreakdownModelShares 返回每个 ids 元素按模型拆开的用量明细（费用降序）。
// ids 为空时直接返回空表，不发起查询。
func (r *usageLogRepository) GetBreakdownModelShares(
	ctx context.Context,
	startTime, endTime time.Time,
	dim usagestats.UserBreakdownDimension,
	modelDim usagestats.BreakdownModelDimension,
	ids []int64,
) (map[int64][]usagestats.BreakdownModelShare, error) {
	column, ok := breakdownModelColumn(modelDim)
	if !ok || len(ids) == 0 {
		return map[int64][]usagestats.BreakdownModelShare{}, nil
	}

	modelExpr := resolveModelDimensionExpressionWithAlias(dim.ModelType, "ul")
	query := `
		SELECT
			` + column + ` as dimension_id,
			` + modelExpr + ` as model,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost
		FROM usage_logs ul
		WHERE ul.created_at >= $1 AND ul.created_at < $2
		  AND ` + column + ` = ANY($3)
	`
	args := []any{startTime, endTime, pq.Array(ids)}
	clause, args := breakdownFilterClause(dim, args)
	query += clause
	query += " GROUP BY " + column + ", model"
	query += " ORDER BY dimension_id, SUM(ul.actual_cost) DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	shares := make(map[int64][]usagestats.BreakdownModelShare, len(ids))
	for rows.Next() {
		var id int64
		var share usagestats.BreakdownModelShare
		if err := rows.Scan(&id, &share.Model, &share.Requests, &share.Tokens, &share.Cost, &share.ActualCost); err != nil {
			return nil, err
		}
		shares[id] = append(shares[id], share)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shares, nil
}
