//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type geminiCacheSettlementFixture struct {
	repo    *geminiCachedContentRepository
	key     *service.APIKey
	account *service.Account
	group   *service.Group
	user    *service.User
}

func newGeminiCacheSettlementFixture(t *testing.T, quota float64) *geminiCacheSettlementFixture {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: "gemini-cache-" + uuid.NewString() + "@example.test"})
	group := mustCreateGroup(t, client, &service.Group{Name: "gemini-cache-" + uuid.NewString(), Platform: service.PlatformGemini, RateMultiplier: 1})
	account := mustCreateAccount(t, client, &service.Account{Name: "gemini-cache-" + uuid.NewString(), Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 100.0}})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, GroupID: &group.ID, Key: "sk-cache-" + uuid.NewString(), Quota: quota, RateLimit5h: 100, RateLimit1d: 100, RateLimit7d: 100})
	// These tests intentionally commit real transactions (including concurrent
	// ones), unlike the rollback-scoped repository suites. Remove only this
	// fixture's rows so later whole-site aggregates keep their clean baseline.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, step := range []struct {
			query string
			id    int64
		}{
			{`DELETE FROM gemini_cached_contents WHERE api_key_id=$1`, key.ID},
			{`DELETE FROM usage_logs WHERE api_key_id=$1`, key.ID},
			{`DELETE FROM usage_billing_dedup WHERE api_key_id=$1`, key.ID},
			{`DELETE FROM usage_billing_dedup_archive WHERE api_key_id=$1`, key.ID},
			{`DELETE FROM api_keys WHERE id=$1`, key.ID},
			{`DELETE FROM account_groups WHERE account_id=$1`, account.ID},
			{`DELETE FROM accounts WHERE id=$1`, account.ID},
			{`DELETE FROM groups WHERE id=$1`, group.ID},
			{`DELETE FROM users WHERE id=$1`, user.ID},
		} {
			if _, err := integrationDB.ExecContext(ctx, step.query, step.id); err != nil {
				t.Errorf("cache fixture cleanup failed (%s): %v", step.query, err)
			}
		}
	})
	return &geminiCacheSettlementFixture{repo: &geminiCachedContentRepository{sql: integrationDB}, key: key, account: account, group: group, user: user}
}
func (f *geminiCacheSettlementFixture) quote(amount float64) (*service.GeminiCachedContent, *service.GeminiCachedContentSettlement) {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	r := &service.GeminiCachedContent{PublicID: id, UserID: f.user.ID, APIKeyID: f.key.ID, GroupID: f.group.ID, AccountID: f.account.ID, UpstreamName: "cachedContents/" + id, Model: "gemini-2.5-pro", UpstreamModel: "models/gemini-2.5-pro", TotalTokenCount: 1000, ExpireTime: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)}
	requestID := service.GeminiCachedContentCreateUsageRequestID(id)
	q := &service.GeminiCachedContentSettlement{EnforceLimits: true, Command: &service.UsageBillingCommand{RequestID: requestID, APIKeyID: f.key.ID, UserID: f.user.ID, AccountID: f.account.ID, AccountType: service.AccountTypeAPIKey, Model: r.Model, InputTokens: 1000, APIKeyQuotaCost: amount, APIKeyRateLimitCost: amount, AccountQuotaCost: amount}, Log: &service.UsageLog{UserID: f.user.ID, APIKeyID: f.key.ID, AccountID: f.account.ID, GroupID: &f.group.ID, RequestID: requestID, Model: r.Model, RequestedModel: r.Model, InputTokens: 1000, InputCost: amount / 2, TotalCost: amount, ActualCost: amount, RateMultiplier: 1, CreatedAt: time.Now()}}
	return r, q
}
func (f *geminiCacheSettlementFixture) balance(t *testing.T) float64 {
	t.Helper()
	var used float64
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `SELECT quota_used FROM api_keys WHERE id=$1`, f.key.ID).Scan(&used))
	return used
}
func geminiCacheCount(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), query, args...).Scan(&n))
	return n
}

func TestGeminiCacheSettlementAtomicCreateAndDedup(t *testing.T) {
	ctx := context.Background()
	f := newGeminiCacheSettlementFixture(t, 1)
	r, q := f.quote(0.25)
	result, err := f.repo.CreateWithSettlement(ctx, r, q)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Positive(t, r.ID)
	require.InDelta(t, 0.25, f.balance(t), 1e-8)
	require.Equal(t, 1, geminiCacheCount(t, `SELECT count(*) FROM usage_logs WHERE api_key_id=$1 AND request_id=$2`, f.key.ID, q.Command.RequestID))
	result, err = f.repo.CreateWithSettlement(ctx, r, q)
	require.NoError(t, err)
	require.False(t, result.Applied)
	require.InDelta(t, 0.25, f.balance(t), 1e-8)
	other, excess := f.quote(0.9)
	_, err = f.repo.CreateWithSettlement(ctx, other, excess)
	require.ErrorIs(t, err, service.ErrAPIKeyQuotaExhausted)
	require.InDelta(t, 0.25, f.balance(t), 1e-8)
	require.Equal(t, 0, geminiCacheCount(t, `SELECT count(*) FROM gemini_cached_contents WHERE public_id=$1`, other.PublicID))
	require.Equal(t, 0, geminiCacheCount(t, `SELECT count(*) FROM usage_billing_dedup WHERE api_key_id=$1 AND request_id=$2`, f.key.ID, excess.Command.RequestID))
}

func TestGeminiCacheSettlementAuditFailureRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	f := newGeminiCacheSettlementFixture(t, 1)
	r, q := f.quote(0.25)
	q.Log.Model = strings.Repeat("x", 1024) // Existing usage-log schema rejects oversized model names.
	_, err := f.repo.CreateWithSettlement(ctx, r, q)
	require.Error(t, err)
	require.Zero(t, f.balance(t))
	require.Equal(t, 0, geminiCacheCount(t, `SELECT count(*) FROM gemini_cached_contents WHERE public_id=$1`, r.PublicID))
	require.Equal(t, 0, geminiCacheCount(t, `SELECT count(*) FROM usage_billing_dedup WHERE api_key_id=$1 AND request_id=$2`, f.key.ID, q.Command.RequestID))
	var accountUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE((extra->>'quota_used')::numeric,0) FROM accounts WHERE id=$1`, f.account.ID).Scan(&accountUsed))
	require.Zero(t, accountUsed)
}

func TestGeminiCacheSettlementConcurrentCreatesCannotOverspend(t *testing.T) {
	ctx := context.Background()
	f := newGeminiCacheSettlementFixture(t, 1)
	start := make(chan struct{})
	done := make(chan error, 2)
	for range 2 {
		r, q := f.quote(0.6)
		go func() { <-start; _, err := f.repo.CreateWithSettlement(ctx, r, q); done <- err }()
	}
	close(start)
	success, rejected := 0, 0
	for range 2 {
		err := <-done
		if err == nil {
			success++
		} else if errors.Is(err, service.ErrAPIKeyQuotaExhausted) {
			rejected++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, rejected)
	require.InDelta(t, 0.6, f.balance(t), 1e-8)
	require.Equal(t, 1, geminiCacheCount(t, `SELECT count(*) FROM gemini_cached_contents WHERE api_key_id=$1`, f.key.ID))
	require.Equal(t, 1, geminiCacheCount(t, `SELECT count(*) FROM usage_logs WHERE api_key_id=$1`, f.key.ID))
}

func TestGeminiCacheSettlementRenewLeaseAndDebitAreAtomic(t *testing.T) {
	ctx := context.Background()
	f := newGeminiCacheSettlementFixture(t, 1)
	r, q := f.quote(0.2)
	_, err := f.repo.CreateWithSettlement(ctx, r, q)
	require.NoError(t, err)
	baseline := r.ExpireTime
	ok, err := f.repo.BeginMutation(ctx, r.ID, baseline, "lease-one", 2*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	_, renew := f.quote(0.3)
	renew.Command.RequestID = service.GeminiCachedContentPatchUsageRequestID(r.PublicID)
	renew.Log.RequestID = renew.Command.RequestID
	renew.Command.InputTokens = 0
	renew.Log.InputTokens = 0
	_, err = f.repo.CommitMutationWithSettlement(ctx, r, "wrong-owner", baseline.Add(time.Hour), renew)
	require.ErrorIs(t, err, service.ErrGeminiCachedContentMutationConflict)
	require.InDelta(t, 0.2, f.balance(t), 1e-8)
	result, err := f.repo.CommitMutationWithSettlement(ctx, r, "lease-one", baseline.Add(time.Hour), renew)
	require.NoError(t, err)
	require.True(t, result.Applied)
	got, err := f.repo.GetForOwner(ctx, f.key.ID, r.PublicID)
	require.NoError(t, err)
	require.True(t, baseline.Add(time.Hour).Equal(got.ExpireTime))
	require.InDelta(t, 0.5, f.balance(t), 1e-8)
	_, err = f.repo.CommitMutationWithSettlement(ctx, r, "lease-one", baseline.Add(time.Hour), renew)
	require.ErrorIs(t, err, service.ErrGeminiCachedContentMutationConflict)
	require.InDelta(t, 0.5, f.balance(t), 1e-8)
	ok, err = f.repo.BeginMutation(ctx, r.ID, got.ExpireTime, "lease-two", 2*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	_, tooMuch := f.quote(0.8)
	tooMuch.Command.RequestID = service.GeminiCachedContentPatchUsageRequestID(r.PublicID)
	tooMuch.Log.RequestID = tooMuch.Command.RequestID
	_, err = f.repo.CommitMutationWithSettlement(ctx, got, "lease-two", got.ExpireTime.Add(time.Hour), tooMuch)
	require.ErrorIs(t, err, service.ErrAPIKeyQuotaExhausted)
	after, err := f.repo.GetForOwner(ctx, f.key.ID, r.PublicID)
	require.NoError(t, err)
	require.True(t, after.ExpireTime.Equal(got.ExpireTime))
	require.InDelta(t, 0.5, f.balance(t), 1e-8)
	// No charge is needed to delete a resource, even if quota has since run out.
	_, err = integrationDB.ExecContext(ctx, `UPDATE api_keys SET status='quota_exhausted',quota_used=quota WHERE id=$1`, f.key.ID)
	require.NoError(t, err)
	require.NoError(t, f.repo.DeleteWithMutation(ctx, got, "lease-two"))
	_, err = f.repo.GetForOwner(ctx, f.key.ID, r.PublicID)
	require.ErrorIs(t, err, service.ErrGeminiCachedContentNotFound)
	require.Equal(t, 2, geminiCacheCount(t, `SELECT count(*) FROM usage_logs WHERE api_key_id=$1`, f.key.ID))
}

func TestGeminiCacheSettlementSpendingWindows(t *testing.T) {
	for _, tc := range []struct {
		name, column, used, start string
		want                      error
	}{
		{"5h", "rate_limit_5h", "usage_5h", "window_5h_start", service.ErrAPIKeyRateLimit5hExceeded},
		{"1d", "rate_limit_1d", "usage_1d", "window_1d_start", service.ErrAPIKeyRateLimit1dExceeded},
		{"7d", "rate_limit_7d", "usage_7d", "window_7d_start", service.ErrAPIKeyRateLimit7dExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGeminiCacheSettlementFixture(t, 10)
			// Identifiers are compile-time constants above, never caller input.
			_, err := integrationDB.ExecContext(context.Background(), `UPDATE api_keys SET `+tc.column+`=1,`+tc.used+`=0.8,`+tc.start+`=NOW() WHERE id=$1`, f.key.ID)
			require.NoError(t, err)
			r, q := f.quote(0.3)
			_, err = f.repo.CreateWithSettlement(context.Background(), r, q)
			require.ErrorIs(t, err, tc.want)
			require.Zero(t, f.balance(t))
			_, err = integrationDB.ExecContext(context.Background(), `UPDATE api_keys SET `+tc.start+`=NOW()-INTERVAL '8 days' WHERE id=$1`, f.key.ID)
			require.NoError(t, err)
			_, err = f.repo.CreateWithSettlement(context.Background(), r, q)
			require.NoError(t, err)
			require.InDelta(t, 0.3, f.balance(t), 1e-8)
		})
	}
}
