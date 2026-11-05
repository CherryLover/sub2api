package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.OpsUpstreamErrorRequestReader = (*opsRepository)(nil)
var _ service.OpsUpstreamErrorRequestSummaryReader = (*opsRepository)(nil)
var _ service.OpsUpstreamErrorRequestTrendReader = (*opsRepository)(nil)

const opsUpstreamEventsJSON = `CASE WHEN jsonb_typeof(e.upstream_errors) = 'array' THEN e.upstream_errors ELSE '[]'::jsonb END`

const opsUpstreamAttempts = `(SELECT COALESCE(jsonb_agg(v.item ORDER BY v.idx), '[]'::jsonb) FROM jsonb_array_elements(` + opsUpstreamEventsJSON + `) WITH ORDINALITY v(item,idx) WHERE jsonb_typeof(v.item) = 'object' AND (v.item ? 'upstream_status_code' OR v.item ? 'message' OR v.item ? 'detail'))`

func opsUpstreamEventStatusSQL(event string) string {
	status := fmt.Sprintf("(%s->>'upstream_status_code')", event)
	return fmt.Sprintf("(CASE WHEN %s ~ '^[0-9]{1,3}$' THEN %s::int ELSE 0 END)", status, status)
}

func opsUpstreamFailureEventSQL(event string) string {
	status := opsUpstreamEventStatusSQL(event)
	return fmt.Sprintf(
		`(%s >= 400 OR (%s = 0 AND COALESCE(%s->>'kind','') <> '' AND (COALESCE(%s->>'message','') <> '' OR COALESCE(%s->>'detail','') <> '')))`,
		status, status, event, event, event,
	)
}

func upstreamErrorRequestsWhere(f *service.OpsUpstreamErrorRequestFilter) (string, []any) {
	args := []any{f.StartTime.UTC(), f.EndTime.UTC()}
	clauses := []string{"e.created_at >= $1", "e.created_at < $2"}
	if f.APIKeyID > 0 {
		args = append(args, f.APIKeyID)
		clauses = append(clauses, fmt.Sprintf("e.api_key_id = $%d", len(args)))
	}
	eventAccountCond := ""
	legacyAccountCond := ""
	if f.AccountID > 0 {
		args = append(args, strconv.FormatInt(f.AccountID, 10))
		p := fmt.Sprintf("$%d", len(args))
		eventAccountCond = " AND ev->>'account_id' = " + p
		legacyAccountCond = " AND e.account_id::text = " + p
	}
	noEvents := "jsonb_array_length(" + opsUpstreamEventsJSON + ") = 0"
	legacyPhase := "LOWER(TRIM(COALESCE(e.error_phase,''))) IN ('upstream','network','account_auth')"
	legacyFailure := "COALESCE(e.upstream_status_code,e.status_code,0) >= 400"
	failureExists := `EXISTS (SELECT 1 FROM jsonb_array_elements(` + opsUpstreamEventsJSON + `) ev WHERE jsonb_typeof(ev) = 'object' AND ` +
		opsUpstreamFailureEventSQL("ev") + eventAccountCond + `)`
	legacyIncluded := `(` + noEvents + ` AND ` + legacyPhase + ` AND ` + legacyFailure + legacyAccountCond + `)`
	clauses = append(clauses, `(`+failureExists+` OR `+legacyIncluded+`)`)
	if f.StatusCode != 0 {
		args = append(args, f.StatusCode)
		p := fmt.Sprintf("$%d", len(args))
		eventStatusMatch := `EXISTS (SELECT 1 FROM jsonb_array_elements(` + opsUpstreamEventsJSON + `) ev WHERE jsonb_typeof(ev) = 'object' AND ` +
			opsUpstreamEventStatusSQL("ev") + ` = ` + p + eventAccountCond + `)`
		legacyStatusMatch := `(` + noEvents + ` AND COALESCE(e.upstream_status_code,e.status_code,0) = ` + p + `)`
		clauses = append(clauses, `(`+eventStatusMatch+` OR `+legacyStatusMatch+`)`)
	}
	if f.FinalStatusCode != 0 {
		args = append(args, f.FinalStatusCode)
		clauses = append(clauses, fmt.Sprintf("e.status_code = $%d", len(args)))
	}
	if f.Outcome == "recovered" {
		clauses = append(clauses, "COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300")
	}
	if f.Outcome == "failed" {
		clauses = append(clauses, "NOT (COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300)")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}
func (r *opsRepository) ListUpstreamErrorRequests(ctx context.Context, f *service.OpsUpstreamErrorRequestFilter) (*service.OpsUpstreamErrorRequestList, error) {
	where, args := upstreamErrorRequestsWhere(f)
	out := &service.OpsUpstreamErrorRequestList{Items: []*service.OpsUpstreamErrorRequest{}, Page: f.Page, PageSize: f.PageSize}
	if err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM ops_error_logs e "+where, args...).Scan(&out.Total); err != nil {
		return nil, err
	}
	query := `WITH page AS (SELECT e.* FROM ops_error_logs e ` + where + fmt.Sprintf(` ORDER BY e.created_at DESC,e.id DESC LIMIT $%d OFFSET $%d)
 SELECT e.id,e.created_at,COALESCE(e.client_request_id,''),COALESCE(e.request_id,''),e.api_key_id,COALESCE(k.name,''),e.account_id,COALESCE(a.name,''),COALESCE(NULLIF(e.requested_model,''),e.model,''),COALESCE(e.reasoning_effort,''),e.request_body_bytes,COALESCE(e.status_code,0),(`, len(args)+1, len(args)+2) + opsUpstreamAttempts + `)::text,e.upstream_status_code,COALESCE(e.upstream_error_message,e.error_message,''),COALESCE(e.upstream_error_detail,''),u.account_id,COALESCE(fa.name,'')
 FROM page e
 LEFT JOIN api_keys k ON k.id=e.api_key_id
 LEFT JOIN accounts a ON a.id=e.account_id
 LEFT JOIN LATERAL (
 SELECT ul.account_id FROM usage_logs ul
 WHERE e.status_code >= 200 AND e.status_code < 300
 AND e.client_request_id IS NOT NULL AND e.client_request_id<>''
 AND ul.request_id='client:' || e.client_request_id AND ul.api_key_id=e.api_key_id
 AND ul.created_at>=e.created_at-INTERVAL '24 hours' AND ul.created_at<=e.created_at+INTERVAL '24 hours'
 ORDER BY ul.created_at DESC,ul.id DESC LIMIT 1
 ) u ON TRUE
 LEFT JOIN accounts fa ON fa.id=u.account_id
 ORDER BY e.created_at DESC,e.id DESC`
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		item := &service.OpsUpstreamErrorRequest{}
		var key, account, final, status sql.NullInt64
		var requestBodyBytes sql.NullInt64
		var raw, message, detail, finalName string
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.ClientRequestID, &item.RequestID, &key, &item.APIKeyName, &account, &item.AccountName, &item.Model, &item.ReasoningEffort, &requestBodyBytes, &item.ClientStatusCode, &raw, &status, &message, &detail, &final, &finalName); err != nil {
			return nil, err
		}
		if requestBodyBytes.Valid {
			v := requestBodyBytes.Int64
			item.RequestBodyBytes = &v
		}
		if key.Valid {
			v := key.Int64
			item.APIKeyID = &v
		}
		if account.Valid {
			v := account.Int64
			item.AccountID = &v
		}
		// Decode independently so one malformed historical event cannot break a page.
		var rawEvents []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &rawEvents); err != nil {
			return nil, err
		}
		item.Attempts = []*service.OpsUpstreamErrorEvent{}
		for _, rawEvent := range rawEvents {
			var event service.OpsUpstreamErrorEvent
			if json.Unmarshal(rawEvent, &event) == nil {
				item.Attempts = append(item.Attempts, &event)
			}
		}
		if len(item.Attempts) == 0 {
			code := int(status.Int64)
			if !status.Valid {
				code = item.ClientStatusCode
			}
			ev := &service.OpsUpstreamErrorEvent{AtUnixMs: item.CreatedAt.UnixMilli(), AccountName: item.AccountName, UpstreamStatusCode: code, Message: message, Detail: detail, Kind: "legacy"}
			if item.AccountID != nil {
				ev.AccountID = *item.AccountID
			}
			item.Attempts = []*service.OpsUpstreamErrorEvent{ev}
		}
		// The event snapshot is authoritative for failure ownership; the stored
		// top-level account can be the eventual successful account in old rows.
		for i := len(item.Attempts) - 1; i >= 0; i-- {
			if item.Attempts[i].AccountID > 0 {
				v := item.Attempts[i].AccountID
				item.AccountID = &v
				item.AccountName = item.Attempts[i].AccountName
				break
			}
		}
		item.Outcome = "failed"
		if item.ClientStatusCode >= 200 && item.ClientStatusCode < 300 {
			item.Outcome = "recovered"
		}
		if final.Valid {
			item.Final = &service.OpsUpstreamErrorRequestFinal{AccountID: final.Int64, AccountName: finalName}
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func buildUpstreamErrorRequestSummarySQL(f *service.OpsUpstreamErrorRequestFilter) (string, []any, string, []any) {
	where, args := upstreamErrorRequestsWhere(f)
	attempts := `(SELECT COUNT(*) FROM jsonb_array_elements(` + opsUpstreamEventsJSON + `) ev WHERE jsonb_typeof(ev) = 'object' AND ` + opsUpstreamFailureEventSQL("ev") + `)`
	summarySQL := `WITH matched AS MATERIALIZED (
 SELECT e.status_code,e.upstream_errors
 FROM ops_error_logs e ` + where + `
)
SELECT COUNT(*),
       COUNT(*) FILTER (WHERE COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300),
       COUNT(*) FILTER (WHERE NOT (COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300)),
       COALESCE(SUM(` + attempts + `),0)
FROM matched e`

	failedFilter := *f
	failedFilter.Outcome = "failed"
	failedWhere, failedArgs := upstreamErrorRequestsWhere(&failedFilter)
	failedSQL := `SELECT e.status_code,COUNT(*)
FROM ops_error_logs e ` + failedWhere + `
 AND e.status_code IS NOT NULL
GROUP BY e.status_code
ORDER BY COUNT(*) DESC,e.status_code ASC
LIMIT 10`
	return summarySQL, args, failedSQL, failedArgs
}

func (r *opsRepository) SummarizeUpstreamErrorRequests(ctx context.Context, f *service.OpsUpstreamErrorRequestFilter) (*service.OpsUpstreamErrorRequestSummary, error) {
	summarySQL, summaryArgs, failedSQL, failedArgs := buildUpstreamErrorRequestSummarySQL(f)
	out := &service.OpsUpstreamErrorRequestSummary{FailedByStatus: []service.OpsUpstreamErrorStatusCount{}}
	if err := r.db.QueryRowContext(ctx, summarySQL, summaryArgs...).Scan(&out.Total, &out.Recovered, &out.Failed, &out.Attempts); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, failedSQL, failedArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item service.OpsUpstreamErrorStatusCount
		if err := rows.Scan(&item.StatusCode, &item.Count); err != nil {
			return nil, err
		}
		out.FailedByStatus = append(out.FailedByStatus, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func buildUpstreamErrorRequestTrendSQL(f *service.OpsUpstreamErrorRequestFilter) (string, []any) {
	where, args := upstreamErrorRequestsWhere(f)
	query := `SELECT TO_CHAR((e.created_at AT TIME ZONE 'Asia/Shanghai')::date, 'YYYY-MM-DD'),
       COUNT(*),
       COUNT(*) FILTER (WHERE COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300),
       COUNT(*) FILTER (WHERE NOT (COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300))
FROM ops_error_logs e ` + where + `
GROUP BY 1
ORDER BY 1 ASC`
	return query, args
}

func (r *opsRepository) GetUpstreamErrorRequestTrend(ctx context.Context, f *service.OpsUpstreamErrorRequestFilter) ([]*service.OpsUpstreamErrorTrendDay, error) {
	query, args := buildUpstreamErrorRequestTrendSQL(f)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]*service.OpsUpstreamErrorTrendDay, 0)
	for rows.Next() {
		item := &service.OpsUpstreamErrorTrendDay{}
		if err := rows.Scan(&item.Date, &item.Total, &item.Recovered, &item.Failed); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
