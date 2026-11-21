package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Lock the credential owner before updating a shadow so a concurrent parent
// reauthorization cannot validate an old probe through an MVCC-only EXISTS check.
// State mutation and scheduler invalidation commit in one statement.
func (r *accountRepository) SetOpenAIOAuthProbeStateIfUnchanged(ctx context.Context, target, owner *service.Account, message string) (bool, error) {
	if target == nil || owner == nil || target.ID <= 0 || owner.ID <= 0 || target.Platform != service.PlatformOpenAI || owner.Platform != service.PlatformOpenAI || target.Type != service.AccountTypeOAuth || owner.Type != service.AccountTypeOAuth || owner.IsCredentialShadow() {
		return false, nil
	}
	if (target.ParentAccountID == nil && target.ID != owner.ID) || (target.ParentAccountID != nil && *target.ParentAccountID != owner.ID) {
		return false, nil
	}
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	targetJSON, err := json.Marshal(normalizeJSONMap(target.Credentials))
	if err != nil {
		return false, err
	}
	ownerJSON, err := json.Marshal(normalizeJSONMap(owner.Credentials))
	if err != nil {
		return false, err
	}
	status := service.StatusActive
	if message != "" {
		status = service.StatusError
	}
	result, err := r.sql.ExecContext(ctx, `
 WITH credential_owner AS (
  SELECT o.id FROM accounts o
  WHERE o.id=$1 AND o.deleted_at IS NULL AND o.platform=$2 AND o.type=$3
   AND o.credentials=$4::jsonb AND o.proxy_id IS NOT DISTINCT FROM $5
   AND o.status=$6 AND COALESCE(o.error_message,'')=$7 AND o.schedulable=$8
   AND o.parent_account_id IS NULL
  FOR UPDATE
 ), updated AS (
  UPDATE accounts AS a SET status=$9,error_message=$10,updated_at=NOW()
  FROM credential_owner o
  WHERE a.id=$11 AND a.deleted_at IS NULL AND a.platform=$2 AND a.type=$3
   AND COALESCE(a.parent_account_id,a.id)=o.id
   AND a.parent_account_id IS NOT DISTINCT FROM $12
   AND a.credentials=$13::jsonb AND a.proxy_id IS NOT DISTINCT FROM $14
   AND a.status=$15 AND COALESCE(a.error_message,'')=$16 AND a.schedulable=$17
  RETURNING a.id
 )
 INSERT INTO scheduler_outbox(event_type,account_id,group_id,payload)
 SELECT $18,updated.id,NULL,NULL FROM updated
 `, owner.ID, service.PlatformOpenAI, service.AccountTypeOAuth, string(ownerJSON), owner.ProxyID,
		owner.Status, owner.ErrorMessage, owner.Schedulable, status, message, target.ID, target.ParentAccountID,
		string(targetJSON), target.ProxyID, target.Status, target.ErrorMessage, target.Schedulable, service.SchedulerOutboxEventAccountChanged)
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
	r.syncSchedulerAccountSnapshotDetached(ctx, target.ID)
	return true, nil
}
