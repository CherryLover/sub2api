package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// withSettlementTx atomically couples cache visibility/expiry, key/account
// charges, deduplication and usage logs. The Tx branch supports the repository's
// integration-test transactions without committing the caller's outer scope.
func (r *geminiCachedContentRepository) withSettlementTx(ctx context.Context, fn func(*sql.Tx) (*service.UsageBillingApplyResult, error)) (*service.UsageBillingApplyResult, error) {
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback() }()
		result, err := fn(tx)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}
	if tx, ok := r.sql.(*sql.Tx); ok {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT gemini_cache_settlement"); err != nil {
			return nil, err
		}
		result, err := fn(tx)
		if err != nil {
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT gemini_cache_settlement")
		}
		_, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT gemini_cache_settlement")
		if err != nil {
			return nil, err
		}
		return result, releaseErr
	}
	return nil, service.ErrGeminiCacheBillingUnavailable
}

func (r *geminiCachedContentRepository) CreateWithSettlement(ctx context.Context, record *service.GeminiCachedContent, q *service.GeminiCachedContentSettlement) (*service.UsageBillingApplyResult, error) {
	if err := service.ValidateGeminiCachedContentSettlement(record, q); err != nil {
		return nil, err
	}
	return r.withSettlementTx(ctx, func(tx *sql.Tx) (*service.UsageBillingApplyResult, error) {
		billing := &usageBillingRepository{}
		q.Command.Normalize()
		applied, err := billing.claimUsageBillingKey(ctx, tx, q.Command)
		if err != nil {
			return nil, err
		}
		boundRepo := &geminiCachedContentRepository{sql: tx}
		if !applied {
			existing, err := boundRepo.GetForOwner(ctx, record.APIKeyID, record.PublicID)
			if err != nil {
				return nil, err
			}
			if existing.AccountID != record.AccountID || existing.GroupID != record.GroupID || existing.UpstreamName != record.UpstreamName {
				return nil, service.ErrUsageBillingRequestConflict
			}
			*record = *existing
			return &service.UsageBillingApplyResult{}, nil
		}
		if err := guardGeminiCacheKey(ctx, tx, record, q); err != nil {
			return nil, err
		}
		if err := boundRepo.Create(ctx, record); err != nil {
			return nil, err
		}
		return applyGeminiCacheSettlement(ctx, tx, billing, q)
	})
}

func (r *geminiCachedContentRepository) CommitMutationWithSettlement(ctx context.Context, record *service.GeminiCachedContent, token string, expiry time.Time, q *service.GeminiCachedContentSettlement) (*service.UsageBillingApplyResult, error) {
	if err := service.ValidateGeminiCachedContentSettlement(record, q); err != nil {
		return nil, err
	}
	return r.withSettlementTx(ctx, func(tx *sql.Tx) (*service.UsageBillingApplyResult, error) {
		if err := lockGeminiCacheMutation(ctx, tx, record, token); err != nil {
			return nil, err
		}
		billing := &usageBillingRepository{}
		q.Command.Normalize()
		applied, err := billing.claimUsageBillingKey(ctx, tx, q.Command)
		if err != nil {
			return nil, err
		}
		if !applied {
			return nil, service.ErrUsageBillingRequestConflict
		}
		if err := guardGeminiCacheKey(ctx, tx, record, q); err != nil {
			return nil, err
		}
		if err := (&geminiCachedContentRepository{sql: tx}).CommitMutation(ctx, record.ID, token, expiry); err != nil {
			return nil, err
		}
		return applyGeminiCacheSettlement(ctx, tx, billing, q)
	})
}

func (r *geminiCachedContentRepository) DeleteWithMutation(ctx context.Context, record *service.GeminiCachedContent, token string) error {
	_, err := r.withSettlementTx(ctx, func(tx *sql.Tx) (*service.UsageBillingApplyResult, error) {
		if err := lockGeminiCacheMutation(ctx, tx, record, token); err != nil {
			return nil, err
		}
		_, err := tx.ExecContext(ctx, `UPDATE gemini_cached_contents SET deleted_at=NOW(),updated_at=NOW(),mutation_token=NULL,mutation_lease_until=NULL WHERE id=$1 AND mutation_token=$2`, record.ID, token)
		return &service.UsageBillingApplyResult{}, err
	})
	return err
}

func lockGeminiCacheMutation(ctx context.Context, tx *sql.Tx, record *service.GeminiCachedContent, token string) error {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM gemini_cached_contents
 WHERE id=$1 AND api_key_id=$2 AND group_id=$3 AND account_id=$4
 AND deleted_at IS NULL AND mutation_token=$5 AND mutation_lease_until>NOW() AND expire_time=$6
 FOR UPDATE`, record.ID, record.APIKeyID, record.GroupID, record.AccountID, token, record.ExpireTime).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrGeminiCachedContentMutationConflict
	}
	return err
}

func applyGeminiCacheSettlement(ctx context.Context, tx *sql.Tx, billing *usageBillingRepository, q *service.GeminiCachedContentSettlement) (*service.UsageBillingApplyResult, error) {
	result := &service.UsageBillingApplyResult{Applied: true}
	if err := billing.applyUsageBillingEffects(ctx, tx, q.Command, result); err != nil {
		return nil, err
	}
	// Use the same full usage-log encoder, inside this transaction rather than
	// its asynchronous batcher. A failed audit write rolls back the charge too.
	inserted, err := (&usageLogRepository{sql: tx}).createSingle(ctx, tx, q.Log)
	if err != nil {
		return nil, err
	}
	if !inserted {
		return nil, service.ErrUsageBillingRequestConflict
	}
	return result, nil
}

// Check the authoritative key row under a row lock, not the auth-cache snapshot.
// This closes concurrent cache renew/create overspending without reinstating the
// full project's Redis/user-balance/subscription reservation subsystem.
func guardGeminiCacheKey(ctx context.Context, tx *sql.Tx, record *service.GeminiCachedContent, q *service.GeminiCachedContentSettlement) error {
	var userID int64
	var groupID sql.NullInt64
	var status string
	var expires sql.NullTime
	var quotaExceeded, fiveHourExceeded, dayExceeded, weekExceeded bool
	keyCost, windowCost := q.Command.APIKeyQuotaCost, q.Command.APIKeyRateLimitCost
	if !q.EnforceLimits {
		keyCost = 0
		windowCost = 0
	}
	err := tx.QueryRowContext(ctx, `SELECT user_id,group_id,status,expires_at,
 ($2::numeric>0 AND quota>0 AND quota_used+$2::numeric>quota),
 ($3::numeric>0 AND rate_limit_5h>0 AND (CASE WHEN window_5h_start IS NULL OR window_5h_start+INTERVAL '5 hours'<=NOW() THEN 0 ELSE usage_5h END)+$3::numeric>rate_limit_5h),
 ($3::numeric>0 AND rate_limit_1d>0 AND (CASE WHEN window_1d_start IS NULL OR window_1d_start+INTERVAL '24 hours'<=NOW() THEN 0 ELSE usage_1d END)+$3::numeric>rate_limit_1d),
 ($3::numeric>0 AND rate_limit_7d>0 AND (CASE WHEN window_7d_start IS NULL OR window_7d_start+INTERVAL '7 days'<=NOW() THEN 0 ELSE usage_7d END)+$3::numeric>rate_limit_7d)
 FROM api_keys WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, record.APIKeyID, keyCost, windowCost).Scan(&userID, &groupID, &status, &expires, &quotaExceeded, &fiveHourExceeded, &dayExceeded, &weekExceeded)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrAPIKeyNotFound
	}
	if err != nil {
		return err
	}
	if userID != record.UserID || !groupID.Valid || groupID.Int64 != record.GroupID {
		return service.ErrGeminiCachedContentNotFound
	}
	if expires.Valid && !expires.Time.After(time.Now()) {
		return service.ErrAPIKeyExpired
	}
	if status != service.StatusAPIKeyActive && !(status == service.StatusAPIKeyQuotaExhausted && keyCost == 0 && windowCost == 0) {
		return service.ErrAPIKeyQuotaExhausted
	}
	if quotaExceeded {
		return service.ErrAPIKeyQuotaExhausted
	}
	if fiveHourExceeded {
		return service.ErrAPIKeyRateLimit5hExceeded
	}
	if dayExceeded {
		return service.ErrAPIKeyRateLimit1dExceeded
	}
	if weekExceeded {
		return service.ErrAPIKeyRateLimit7dExceeded
	}
	return nil
}
