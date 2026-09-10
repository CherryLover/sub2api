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

var _ service.OpsAccountErrorSummaryReader = (*opsRepository)(nil)

const opsAccountRecentErrorMessageMaxRunes = 300

// opsAccountRecentErrorWhere selects an account's error rows in the window.
// A row counts as an error when either the client-facing status or the
// upstream status is >= 400: failover and SSE-level upstream failures are
// logged with a 2xx client status but a 4xx/5xx upstream status.
// Served by idx_ops_error_logs_account_time.
const opsAccountRecentErrorWhere = `
WHERE account_id = ANY($1)
  AND created_at >= $2
  AND NOT COALESCE(is_count_tokens, false)
  AND (COALESCE(status_code, 0) >= 400 OR COALESCE(upstream_status_code, 0) >= 400)`

// The status bucket prefers the upstream status; an explicitly recorded
// upstream status of 0 (no upstream response) falls back to the client status.
const opsAccountRecentErrorCountsSQL = `
SELECT
  account_id,
  COALESCE(NULLIF(upstream_status_code, 0), status_code, 0) AS status_code,
  COUNT(*) AS cnt
FROM ops_error_logs` + opsAccountRecentErrorWhere + `
GROUP BY 1, 2
ORDER BY 1, 3 DESC, 2 ASC`

const opsAccountRecentErrorLastSQL = `
SELECT DISTINCT ON (account_id)
  account_id,
  created_at,
  COALESCE(status_code, 0) AS status_code,
  upstream_status_code,
  COALESCE(NULLIF(upstream_error_message, ''), error_message, '') AS message,
  COALESCE(NULLIF(requested_model, ''), model, '') AS model
FROM ops_error_logs` + opsAccountRecentErrorWhere + `
ORDER BY account_id, created_at DESC, id DESC`

// GetAccountRecentErrorSummaries implements service.OpsAccountErrorSummaryReader.
// Accounts without error rows in the window are absent from the result.
func (r *opsRepository) GetAccountRecentErrorSummaries(ctx context.Context, accountIDs []int64, since time.Time) (map[int64]*service.OpsAccountRecentErrorSummary, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	out := make(map[int64]*service.OpsAccountRecentErrorSummary, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	args := []any{pq.Array(accountIDs), since.UTC()}

	rows, err := r.db.QueryContext(ctx, opsAccountRecentErrorCountsSQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var accountID int64
		var statusCode int
		var count int64
		if err := rows.Scan(&accountID, &statusCode, &count); err != nil {
			return nil, err
		}
		summary := out[accountID]
		if summary == nil {
			summary = &service.OpsAccountRecentErrorSummary{}
			out[accountID] = summary
		}
		summary.Total += count
		summary.ByStatus = append(summary.ByStatus, service.OpsAccountRecentErrorStatusCount{
			StatusCode: statusCode,
			Count:      count,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	if len(out) == 0 {
		return out, nil
	}

	lastRows, err := r.db.QueryContext(ctx, opsAccountRecentErrorLastSQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lastRows.Close() }()
	for lastRows.Next() {
		var (
			accountID    int64
			createdAt    time.Time
			statusCode   int
			upstreamCode sql.NullInt64
			message      string
			model        string
		)
		if err := lastRows.Scan(&accountID, &createdAt, &statusCode, &upstreamCode, &message, &model); err != nil {
			return nil, err
		}
		summary := out[accountID]
		if summary == nil {
			// Counted and last queries are separate statements; a row that
			// landed in between is reflected in neither total nor last.
			continue
		}
		last := &service.OpsAccountRecentError{
			At:         createdAt.UTC(),
			StatusCode: statusCode,
			Message:    truncateOpsAccountRecentErrorMessage(message),
			Model:      strings.TrimSpace(model),
		}
		if upstreamCode.Valid {
			code := int(upstreamCode.Int64)
			last.UpstreamStatusCode = &code
		}
		summary.Last = last
	}
	if err := lastRows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func truncateOpsAccountRecentErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if len(runes) <= opsAccountRecentErrorMessageMaxRunes {
		return message
	}
	return string(runes[:opsAccountRecentErrorMessageMaxRunes])
}
