//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuplicateAccountExtraStripsMonthlyRuntime(t *testing.T) {
	source := map[string]any{"quota_monthly_limit": 100.0, "quota_monthly_used": 23.5, "quota_monthly_start": "2026-01-01T00:00:00Z", "quota_monthly_reset_at": "2026-02-01T00:00:00Z", "quota_monthly_reset_mode": "fixed", "quota_monthly_reset_day": 31, "quota_monthly_reset_hour": 23, "quota_reset_timezone": "Asia/Shanghai", "quota_monthly_prorate": true, "quota_monthly_prorated_limit": 42.5, "quota_monthly_prorated_until": "2026-02-01T00:00:00Z"}
	cloned, err := duplicateAccountExtra(source)
	require.NoError(t, err)
	for _, key := range []string{"quota_monthly_used", "quota_monthly_start", "quota_monthly_reset_at", "quota_monthly_prorated_limit", "quota_monthly_prorated_until"} {
		require.NotContains(t, cloned, key)
		require.Contains(t, source, key)
	}
	for _, key := range []string{"quota_monthly_limit", "quota_monthly_reset_mode", "quota_monthly_reset_day", "quota_monthly_reset_hour", "quota_reset_timezone", "quota_monthly_prorate"} {
		require.EqualValues(t, source[key], cloned[key])
	}
}

func TestUpdateAccountMonthlyQuotaPreservesUsage(t *testing.T) {
	for _, mode := range []string{"rolling", "fixed"} {
		t.Run(mode, func(t *testing.T) {
			repo := newDuplicateAccountRepoStub()
			start := time.Now().UTC().Format(time.RFC3339)
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: StatusActive, Extra: map[string]any{"quota_monthly_limit": 100.0, "quota_monthly_used": 23.5, "quota_monthly_start": start, "quota_monthly_reset_mode": mode, "quota_monthly_reset_day": 1, "quota_monthly_reset_hour": 0, "quota_reset_timezone": "UTC"}}
			repo.accounts[1] = account
			repo.mockAccountRepoForGemini.accountsByID[1] = account
			svc := &adminServiceImpl{accountRepo: repo}
			updated, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{"quota_monthly_limit": 200.0, "quota_monthly_used": 0.0, "quota_monthly_start": "2000-01-01T00:00:00Z", "quota_monthly_reset_mode": mode, "quota_monthly_reset_day": 1, "quota_monthly_reset_hour": 0, "quota_reset_timezone": "UTC"}})
			require.NoError(t, err)
			require.Equal(t, 200.0, updated.Extra["quota_monthly_limit"])
			require.Equal(t, 23.5, updated.Extra["quota_monthly_used"])
			require.Equal(t, start, updated.Extra["quota_monthly_start"])
		})
	}
}

func TestUpdateAccountMonthlyQuotaPreservesProrationOnUnrelatedExtraEdit(t *testing.T) {
	repo := newDuplicateAccountRepoStub()
	start := time.Now().UTC()
	until := start.Add(12 * 24 * time.Hour).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Extra: map[string]any{
			"quota_monthly_limit":          100.0,
			"quota_monthly_used":           23.5,
			"quota_monthly_start":          start.Format(time.RFC3339),
			"quota_monthly_reset_mode":     "fixed",
			"quota_monthly_reset_day":      1,
			"quota_monthly_reset_hour":     0,
			"quota_reset_timezone":         "UTC",
			"quota_monthly_prorate":        true,
			"quota_monthly_prorated_limit": 42.5,
			"quota_monthly_prorated_until": until,
		},
	}
	repo.accounts[1] = account
	repo.mockAccountRepoForGemini.accountsByID[1] = account
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		"quota_monthly_limit":      100.0,
		"quota_monthly_reset_mode": "fixed",
		"quota_monthly_reset_day":  1,
		"quota_monthly_reset_hour": 0,
		"quota_reset_timezone":     "UTC",
		"quota_monthly_prorate":    true,
		"unrelated_edit":           "kept",
	}})
	require.NoError(t, err)
	require.Equal(t, 42.5, updated.Extra["quota_monthly_prorated_limit"])
	require.Equal(t, until, updated.Extra["quota_monthly_prorated_until"])
	require.Equal(t, "kept", updated.Extra["unrelated_edit"])
}

func TestUpdateAccountMonthlyQuotaRecalculatesWhenProrationEnabled(t *testing.T) {
	repo := newDuplicateAccountRepoStub()
	start := time.Now().UTC()
	account := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Extra: map[string]any{
			"quota_monthly_limit":      100.0,
			"quota_monthly_used":       23.5,
			"quota_monthly_start":      start.Format(time.RFC3339),
			"quota_monthly_reset_mode": "fixed",
			"quota_monthly_reset_day":  1,
			"quota_monthly_reset_hour": 0,
			"quota_reset_timezone":     "UTC",
			"quota_monthly_prorate":    false,
		},
	}
	repo.accounts[1] = account
	repo.mockAccountRepoForGemini.accountsByID[1] = account
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{
		"quota_monthly_limit":      100.0,
		"quota_monthly_reset_mode": "fixed",
		"quota_monthly_reset_day":  1,
		"quota_monthly_reset_hour": 0,
		"quota_reset_timezone":     "UTC",
		"quota_monthly_prorate":    true,
	}})
	require.NoError(t, err)
	proratedLimit, ok := updated.Extra["quota_monthly_prorated_limit"].(float64)
	require.True(t, ok)
	require.Greater(t, proratedLimit, 0.0)
	require.LessOrEqual(t, proratedLimit, 100.0)
	proratedUntil, err := time.Parse(time.RFC3339, updated.Extra["quota_monthly_prorated_until"].(string))
	require.NoError(t, err)
	require.True(t, proratedUntil.After(time.Now()))
}

func TestBuildAccountForCreateComputesMonthlyProration(t *testing.T) {
	extra := map[string]any{
		"quota_monthly_limit":      100.0,
		"quota_monthly_reset_mode": "fixed",
		"quota_monthly_reset_day":  1,
		"quota_monthly_reset_hour": 0,
		"quota_reset_timezone":     "Asia/Shanghai",
		"quota_monthly_prorate":    true,
	}
	account, err := buildAccountForCreate(&CreateAccountInput{
		Name:     "monthly-prorate",
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    extra,
	}, extra)
	require.NoError(t, err)
	proratedLimit, ok := account.Extra["quota_monthly_prorated_limit"].(float64)
	require.True(t, ok)
	require.Greater(t, proratedLimit, 0.0)
	require.LessOrEqual(t, proratedLimit, 100.0)
	require.Equal(t, account.Extra["quota_monthly_reset_at"], account.Extra["quota_monthly_prorated_until"])
}
