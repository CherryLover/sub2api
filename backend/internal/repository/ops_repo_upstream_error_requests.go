package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.OpsUpstreamErrorRequestReader = (*opsRepository)(nil)

const opsUpstreamAttempts = `(SELECT COALESCE(jsonb_agg(v.item ORDER BY v.idx), '[]'::jsonb) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(e.upstream_errors) = 'array' THEN e.upstream_errors ELSE '[]'::jsonb END) WITH ORDINALITY v(item,idx) WHERE jsonb_typeof(v.item) = 'object' AND (v.item ? 'upstream_status_code' OR v.item ? 'message' OR v.item ? 'detail'))`

func upstreamErrorRequestsWhere(f *service.OpsUpstreamErrorRequestFilter) (string, []any) {
	args := []any{f.StartTime.UTC(), f.EndTime.UTC()}
	clauses := []string{"e.created_at >= $1", "e.created_at < $2"}
	codes := "('502','503')"
	if f.StatusCode != 0 {
		args = append(args, fmt.Sprint(f.StatusCode))
		codes = fmt.Sprintf("($%d)", len(args))
	}
	// Match any attempt, not merely the last upstream status. Only legacy rows
	// with no events fall back to the single top-level upstream status.
	eventCondition := "ev->>'upstream_status_code' IN " + codes
	legacyCondition := "COALESCE(e.upstream_status_code,e.status_code,0)::text IN " + codes
	if f.APIKeyID > 0 {
		args = append(args, f.APIKeyID)
		clauses = append(clauses, fmt.Sprintf("e.api_key_id = $%d", len(args)))
	}
	if f.AccountID > 0 {
		args = append(args, fmt.Sprint(f.AccountID))
		p := fmt.Sprintf("$%d", len(args))
		eventCondition += " AND ev->>'account_id' = " + p
		legacyCondition += " AND e.account_id::text = " + p
	}
	clauses = append(clauses, `(EXISTS (SELECT 1 FROM jsonb_array_elements(`+opsUpstreamAttempts+`) ev WHERE `+eventCondition+`) OR (jsonb_array_length(`+opsUpstreamAttempts+`) = 0 AND `+legacyCondition+`))`)
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
 SELECT e.id,e.created_at,COALESCE(e.client_request_id,''),COALESCE(e.request_id,''),e.api_key_id,COALESCE(k.name,''),e.account_id,COALESCE(a.name,''),COALESCE(NULLIF(e.requested_model,''),e.model,''),COALESCE(e.status_code,0),(`, len(args)+1, len(args)+2) + opsUpstreamAttempts + `)::text,e.upstream_status_code,COALESCE(e.upstream_error_message,e.error_message,''),COALESCE(e.upstream_error_detail,''),u.account_id,COALESCE(fa.name,'')
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
		var raw, message, detail, finalName string
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.ClientRequestID, &item.RequestID, &key, &item.APIKeyName, &account, &item.AccountName, &item.Model, &item.ClientStatusCode, &raw, &status, &message, &detail, &final, &finalName); err != nil {
			return nil, err
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
