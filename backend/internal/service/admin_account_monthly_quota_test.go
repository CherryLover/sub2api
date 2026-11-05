//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuplicateAccountExtraStripsMonthlyRuntime(t *testing.T) {
	source := map[string]any{"quota_monthly_limit": 100.0, "quota_monthly_used": 23.5, "quota_monthly_start": "2026-01-01T00:00:00Z", "quota_monthly_reset_at": "2026-02-01T00:00:00Z", "quota_monthly_reset_mode": "fixed", "quota_monthly_reset_day": 31, "quota_monthly_reset_hour": 23, "quota_reset_timezone": "Asia/Shanghai"}
	cloned, err := duplicateAccountExtra(source)
	require.NoError(t, err)
	for _, key := range []string{"quota_monthly_used", "quota_monthly_start", "quota_monthly_reset_at"} {
		require.NotContains(t, cloned, key)
		require.Contains(t, source, key)
	}
	for _, key := range []string{"quota_monthly_limit", "quota_monthly_reset_mode", "quota_monthly_reset_day", "quota_monthly_reset_hour", "quota_reset_timezone"} {
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
