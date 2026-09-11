package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.OpsRequestChainReader = (*opsRepository)(nil)

// opsRequestChainUsageLookupSlack 是 usage_logs 关联时在错误行时间窗两侧各放宽的量。
//
// usage_logs 在部分部署里是按 created_at 分区的（migrations/035）。不带时间条件的
// request_id 等值查询会把所有分区都翻一遍；给一个宽松的时间范围就能让规划器剪枝。
// 放得这么宽是刻意的：计费落库与错误落库之间只差几秒，一天的余量绝不会把真实的
// usage 行排除在外——宁可多扫一个分区，也不要漏掉最终结果去回一个假的 final: null。
const opsRequestChainUsageLookupSlack = 24 * time.Hour

// opsRequestChainErrorsSQL 取该 client_request_id 的全部 ops_error_logs 记录，
// 并把 upstream_errors 这个 JSONB 数组就地展开成一次一行。
//
// 几个必须这么写的地方：
//
//   - jsonb_array_elements 遇到非数组会直接报错，而 upstream_errors 可能是 NULL、
//     也可能是 JSON null，所以先用 jsonb_typeof 判一次，不是数组就喂 '[]'。
//   - 用 LEFT JOIN LATERAL ... ON TRUE 而不是 CROSS JOIN：数组为空的错误记录
//     （只有客户端侧失败、压根没打到上游）也必须保留，否则整条链路会凭空消失。
//   - 数字字段同样先 jsonb_typeof 判型再转：一条脏数据不应该让整个接口 500。
//   - WITH ORDINALITY 保住数组内的原始顺序，用于时间戳缺失时的稳定排序。
//
// 账号名优先用事件里记下的快照（账号后来改名/删号也能解释历史），快照为空才回退
// 到 accounts 表。
const opsRequestChainErrorsSQL = `
SELECT
  e.id,
  e.created_at,
  COALESCE(e.status_code, 0) AS status_code,
  COALESCE(e.model, '') AS model,
  COALESCE(e.requested_model, '') AS requested_model,
  COALESCE(e.stream, false) AS stream,
  e.time_to_first_token_ms,
  ev.idx AS attempt_index,
  CASE WHEN jsonb_typeof(ev.item->'at_unix_ms') = 'number'
       THEN (ev.item->>'at_unix_ms')::BIGINT END AS at_unix_ms,
  CASE WHEN jsonb_typeof(ev.item->'account_id') = 'number'
       THEN (ev.item->>'account_id')::BIGINT END AS attempt_account_id,
  COALESCE(NULLIF(ev.item->>'account_name', ''), a.name, '') AS attempt_account_name,
  COALESCE(NULLIF(ev.item->>'platform', ''), NULLIF(e.platform, ''), '') AS attempt_platform,
  CASE WHEN jsonb_typeof(ev.item->'upstream_status_code') = 'number'
       THEN (ev.item->>'upstream_status_code')::INT END AS attempt_upstream_status_code,
  COALESCE(ev.item->>'message', '') AS attempt_message,
  COALESCE(ev.item->>'kind', '') AS attempt_kind
FROM ops_error_logs e
LEFT JOIN LATERAL jsonb_array_elements(
  CASE WHEN jsonb_typeof(e.upstream_errors) = 'array' THEN e.upstream_errors ELSE '[]'::jsonb END
) WITH ORDINALITY AS ev(item, idx) ON TRUE
LEFT JOIN accounts a ON a.id = CASE WHEN jsonb_typeof(ev.item->'account_id') = 'number'
                                    THEN (ev.item->>'account_id')::BIGINT END
WHERE e.client_request_id = $1
ORDER BY e.created_at ASC, e.id ASC, ev.idx ASC`

// opsRequestChainUsageSQL 关联最终成功结果。
//
// $1 必须是「client:」+ 裸 client_request_id：网关写 usage_logs 时用的是带前缀的
// 幂等键（gateway_usage_billing.go 的 resolveUsageBillingRequestID），
// 直接拿 ops_error_logs.client_request_id 去比会一条都对不上。
//
// usage_logs 没有 total_tokens 列，四类 token 现加，与仓库其它统计口径一致。
// 理论上 (request_id, api_key_id) 上有唯一索引，一次客户端请求至多一行；
// 万一有多行（历史数据），取最新那条。
const opsRequestChainUsageSQL = `
SELECT
  ul.account_id,
  COALESCE(a.name, '') AS account_name,
  (ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens) AS total_tokens,
  ul.total_cost,
  ul.first_token_ms
FROM usage_logs ul
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.request_id = $1
  AND ul.created_at >= $2
  AND ul.created_at <= $3
ORDER BY ul.created_at DESC, ul.id DESC
LIMIT 1`

// GetRequestChainSource implements service.OpsRequestChainReader.
func (r *opsRepository) GetRequestChainSource(ctx context.Context, clientRequestID string) (*service.OpsRequestChainSource, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	clientRequestID = strings.TrimSpace(clientRequestID)
	if clientRequestID == "" {
		return nil, fmt.Errorf("empty client request id")
	}

	records, err := r.queryOpsRequestChainErrors(ctx, clientRequestID)
	if err != nil {
		return nil, err
	}
	source := &service.OpsRequestChainSource{Errors: records}
	if len(records) == 0 {
		// 没有错误记录就没有链路，不必再去翻 usage_logs。
		return source, nil
	}

	usage, err := r.queryOpsRequestChainUsage(ctx, clientRequestID, records)
	if err != nil {
		return nil, err
	}
	source.Usage = usage
	return source, nil
}

func (r *opsRepository) queryOpsRequestChainErrors(ctx context.Context, clientRequestID string) ([]*service.OpsRequestChainErrorRecord, error) {
	rows, err := r.db.QueryContext(ctx, opsRequestChainErrorsSQL, clientRequestID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	records := make([]*service.OpsRequestChainErrorRecord, 0, 4)
	byID := make(map[int64]*service.OpsRequestChainErrorRecord, 4)

	for rows.Next() {
		var (
			id             int64
			createdAt      time.Time
			statusCode     int
			model          string
			requestedModel string
			stream         bool
			ttft           sql.NullInt64

			attemptIndex      sql.NullInt64
			atUnixMs          sql.NullInt64
			attemptAccountID  sql.NullInt64
			attemptAccount    string
			attemptPlatform   string
			attemptUpstream   sql.NullInt64
			attemptMessage    string
			attemptKindString string
		)
		if err := rows.Scan(
			&id,
			&createdAt,
			&statusCode,
			&model,
			&requestedModel,
			&stream,
			&ttft,
			&attemptIndex,
			&atUnixMs,
			&attemptAccountID,
			&attemptAccount,
			&attemptPlatform,
			&attemptUpstream,
			&attemptMessage,
			&attemptKindString,
		); err != nil {
			return nil, err
		}

		record := byID[id]
		if record == nil {
			record = &service.OpsRequestChainErrorRecord{
				ID:             id,
				CreatedAt:      createdAt.UTC(),
				StatusCode:     statusCode,
				Model:          strings.TrimSpace(model),
				RequestedModel: strings.TrimSpace(requestedModel),
				Stream:         stream,
				Attempts:       []*service.OpsRequestChainAttemptRecord{},
			}
			if ttft.Valid {
				value := ttft.Int64
				record.TimeToFirstTokenMs = &value
			}
			byID[id] = record
			records = append(records, record)
		}

		// attempt_index 为 NULL 说明 upstream_errors 是 NULL / 非数组 / 空数组，
		// 这一行只是被 LEFT JOIN 保留下来的错误记录本身，没有尝试可加。
		if !attemptIndex.Valid {
			continue
		}
		attempt := &service.OpsRequestChainAttemptRecord{
			AccountName: strings.TrimSpace(attemptAccount),
			Platform:    strings.TrimSpace(attemptPlatform),
			Message:     strings.TrimSpace(attemptMessage),
			Kind:        strings.TrimSpace(attemptKindString),
		}
		if atUnixMs.Valid {
			attempt.AtUnixMs = atUnixMs.Int64
		}
		if attemptAccountID.Valid {
			attempt.AccountID = attemptAccountID.Int64
		}
		if attemptUpstream.Valid {
			attempt.UpstreamStatusCode = int(attemptUpstream.Int64)
		}
		record.Attempts = append(record.Attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *opsRepository) queryOpsRequestChainUsage(
	ctx context.Context,
	clientRequestID string,
	records []*service.OpsRequestChainErrorRecord,
) (*service.OpsRequestChainUsageRecord, error) {
	windowStart, windowEnd := opsRequestChainUsageWindow(records)

	var (
		accountID    sql.NullInt64
		accountName  string
		totalTokens  sql.NullInt64
		cost         sql.NullFloat64
		firstTokenMs sql.NullInt64
	)
	err := r.db.QueryRowContext(
		ctx,
		opsRequestChainUsageSQL,
		service.OpsUsageLogRequestIDForClientRequestID(clientRequestID),
		windowStart,
		windowEnd,
	).Scan(&accountID, &accountName, &totalTokens, &cost, &firstTokenMs)
	if err != nil {
		if err == sql.ErrNoRows {
			// 关联不到就是关联不到：服务层会回 "final": null，不编造最终结果。
			return nil, nil
		}
		return nil, err
	}

	usage := &service.OpsRequestChainUsageRecord{
		AccountID:   accountID.Int64,
		AccountName: strings.TrimSpace(accountName),
		TotalTokens: totalTokens.Int64,
		Cost:        cost.Float64,
	}
	if firstTokenMs.Valid {
		value := firstTokenMs.Int64
		usage.FirstTokenMs = &value
	}
	return usage, nil
}

// opsRequestChainUsageWindow 由错误记录的时间跨度加固定余量得到 usage_logs 的查询窗口。
func opsRequestChainUsageWindow(records []*service.OpsRequestChainErrorRecord) (time.Time, time.Time) {
	var start, end time.Time
	for _, record := range records {
		if record == nil || record.CreatedAt.IsZero() {
			continue
		}
		if start.IsZero() || record.CreatedAt.Before(start) {
			start = record.CreatedAt
		}
		if end.IsZero() || record.CreatedAt.After(end) {
			end = record.CreatedAt
		}
	}
	if start.IsZero() {
		start = time.Now().UTC()
	}
	if end.IsZero() {
		end = start
	}
	return start.Add(-opsRequestChainUsageLookupSlack).UTC(), end.Add(opsRequestChainUsageLookupSlack).UTC()
}
