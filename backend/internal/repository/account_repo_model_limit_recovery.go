package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ClearModelRateLimitSnapshotIfUnchanged removes exactly the observed scope.
// Deletion and scheduler invalidation are one PostgreSQL statement: a failed
// outbox insert cannot silently leave scheduling and durable state divergent.
func (r *accountRepository) ClearModelRateLimitSnapshotIfUnchanged(ctx context.Context, id int64, scope string, observed map[string]any) (bool, error) {
	if id <= 0 || strings.TrimSpace(scope) == "" || len(observed) == 0 {
		return false, nil
	}
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	expected, err := json.Marshal(observed)
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
  WITH updated AS (
   UPDATE accounts AS a
   SET extra = COALESCE(a.extra, '{}'::jsonb) #- ARRAY['model_rate_limits', $1::text], updated_at = NOW()
   WHERE a.id = $2 AND a.deleted_at IS NULL AND a.platform = $4
    AND COALESCE(a.extra, '{}'::jsonb) #> ARRAY['model_rate_limits', $1::text] = $3::jsonb
   RETURNING a.id
  )
  INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
  SELECT $5, updated.id, NULL, NULL FROM updated
 `, scope, id, string(expected), service.PlatformAnthropic, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}
