package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var _ service.OpsAccountErrorOutcomeReader = (*opsRepository)(nil)

// opsAccountRecentErrorStatusBucketExpr 必须与 opsAccountRecentErrorCountsSQL 里的
// 分桶表达式逐字一致，否则两条语句分出来的档位对不上号，拆分会挂到错误的状态码上。
// ops_repo_account_error_outcomes_test.go 里有一条断言盯着这个一致性。
const opsAccountRecentErrorStatusBucketExpr = "COALESCE(NULLIF(upstream_status_code, 0), status_code, 0)"

// opsAccountRecentErrorRecoveredPredicate 是「已恢复」的 SQL 口径：客户端最终看到的
// HTTP 码属于 2xx，说明上游那几次报错已经被网关的重试/换号自动兜住了。
//
// 两个 FILTER 都用 COALESCE(status_code, 0) 兜底，所以互为补集、加起来正好是 COUNT(*)；
// status_code 为 NULL（没记到客户端状态）时落进 failed —— 没看到 2xx 就不声称成功。
const opsAccountRecentErrorRecoveredPredicate = "COALESCE(status_code, 0) BETWEEN 200 AND 299"

// opsAccountRecentErrorOutcomeSQL 复用 opsAccountRecentErrorWhere（同一份 WHERE，
// 同一批行，命中 idx_ops_error_logs_account_time），只多算三样东西：
//
//   - recovered / failed：按客户端最终状态码拆分的两个互补计数。
//   - last_created_at：本档最近一行的时间，服务层用它判断「账号级最近那一行」属于哪一档。
//   - last_client_request_id：本档最近一行的 client_request_id，前端拿它开「请求链路」面板。
//
// array_agg + ORDER BY 取第一个元素，等价于对每个分组做一次 DISTINCT ON，
// 但不需要再开一条语句。
const opsAccountRecentErrorOutcomeSQL = `
SELECT
  account_id,
  ` + opsAccountRecentErrorStatusBucketExpr + ` AS status_code,
  COUNT(*) FILTER (WHERE ` + opsAccountRecentErrorRecoveredPredicate + `) AS recovered,
  COUNT(*) FILTER (WHERE NOT (` + opsAccountRecentErrorRecoveredPredicate + `)) AS failed,
  MAX(created_at) AS last_created_at,
  (array_agg(COALESCE(client_request_id, '') ORDER BY created_at DESC, id DESC))[1] AS last_client_request_id
FROM ops_error_logs` + opsAccountRecentErrorWhere + `
GROUP BY 1, 2
ORDER BY 1, 2`

// GetAccountRecentErrorOutcomes implements service.OpsAccountErrorOutcomeReader.
// Accounts without error rows in the window are absent from the result.
func (r *opsRepository) GetAccountRecentErrorOutcomes(ctx context.Context, accountIDs []int64, since time.Time) (map[int64]*service.OpsAccountRecentErrorOutcome, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	out := make(map[int64]*service.OpsAccountRecentErrorOutcome, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.QueryContext(ctx, opsAccountRecentErrorOutcomeSQL, pq.Array(accountIDs), since.UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			accountID     int64
			statusCode    int
			recovered     int64
			failed        int64
			lastCreatedAt sql.NullTime
			lastRequestID sql.NullString
		)
		if err := rows.Scan(&accountID, &statusCode, &recovered, &failed, &lastCreatedAt, &lastRequestID); err != nil {
			return nil, err
		}
		outcome := out[accountID]
		if outcome == nil {
			outcome = &service.OpsAccountRecentErrorOutcome{}
			out[accountID] = outcome
		}
		outcome.Recovered += recovered
		outcome.Failed += failed
		row := service.OpsAccountRecentErrorStatusOutcome{
			StatusCode:          statusCode,
			Recovered:           recovered,
			Failed:              failed,
			LastClientRequestID: strings.TrimSpace(lastRequestID.String),
		}
		if lastCreatedAt.Valid {
			row.LastCreatedAt = lastCreatedAt.Time.UTC()
		}
		outcome.ByStatus = append(outcome.ByStatus, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
