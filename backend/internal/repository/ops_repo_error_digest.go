package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// opsErrorDigestBreakdownRowLimit 一次汇总最多取回的分组行数。
//
// 正常规模下行数远到不了这个量级（密钥数 × 错误类型 × 状态码），设上限只是防止
// 某天上游把状态码打散成几百种时把一整张表拉进内存。按 count 倒序截断，
// 被丢掉的一定是长尾，正文里排在前面的分组不受影响。
const opsErrorDigestBreakdownRowLimit = 5000

// GetErrorDigestBreakdown 取一个时间区间内的报错计数，
// 按「密钥 × 错误类型 × 状态码 × 是否业务拦截」分组。
//
// 只发一条 SQL：定时汇总要先按密钥分组、每组再挑出现最多的几种错误类型，这些折叠全放在
// Go 侧做。否则"每个密钥的前 3 种错误"就得按密钥数回查，变成 N+1。
//
// 状态码进分组键，是因为正文要写成「上游过载(529) 30」——只有错误类型名说不清是哪种过载；
// is_business_limited 进分组键，是因为业务拦截（余额不足、额度用尽）每天都有一堆，
// 必须和真故障分开计数，否则真故障会被淹没。
//
// 时间条件走 idx_ops_error_logs_created_at。users / api_keys 用 LEFT JOIN 且刻意不过滤
// deleted_at：软删掉的用户和密钥同样要能显示名字；真取不到名字时由上层回退成 ID。
func (r *opsRepository) GetErrorDigestBreakdown(ctx context.Context, start, end time.Time) ([]*service.OpsErrorDigestRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("start/end required")
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end must be after start")
	}

	q := `
SELECT
  e.api_key_id,
  e.user_id,
  COALESCE(u.username, ''),
  COALESCE(u.email, ''),
  COALESCE(ak.name, ''),
  COALESCE(e.error_type, ''),
  COALESCE(e.upstream_status_code, e.status_code, 0) AS status_code,
  e.is_business_limited,
  COUNT(*) AS cnt
FROM ops_error_logs e
LEFT JOIN users u ON u.id = e.user_id
LEFT JOIN api_keys ak ON ak.id = e.api_key_id
WHERE e.created_at >= $1 AND e.created_at < $2
GROUP BY
  e.api_key_id,
  e.user_id,
  u.username,
  u.email,
  ak.name,
  e.error_type,
  COALESCE(e.upstream_status_code, e.status_code, 0),
  e.is_business_limited
ORDER BY cnt DESC
LIMIT ` + itoa(opsErrorDigestBreakdownRowLimit)

	rows, err := r.db.QueryContext(ctx, q, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsErrorDigestRow, 0, 64)
	for rows.Next() {
		var (
			apiKeyID sql.NullInt64
			userID   sql.NullInt64
			item     service.OpsErrorDigestRow
		)
		if err := rows.Scan(
			&apiKeyID,
			&userID,
			&item.Username,
			&item.UserEmail,
			&item.APIKeyName,
			&item.ErrorType,
			&item.StatusCode,
			&item.IsBusinessLimited,
			&item.Count,
		); err != nil {
			return nil, err
		}
		if apiKeyID.Valid {
			id := apiKeyID.Int64
			item.APIKeyID = &id
		}
		if userID.Valid {
			id := userID.Int64
			item.UserID = &id
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
