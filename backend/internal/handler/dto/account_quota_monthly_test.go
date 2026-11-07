package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountFromServiceShallowMonthlyQuota(t *testing.T) {
	for _, typ := range []string{service.AccountTypeAPIKey, service.AccountTypeBedrock, service.AccountTypeServiceAccount} {
		t.Run(typ, func(t *testing.T) {
			extra := map[string]any{"quota_monthly_limit": 100.0, "quota_monthly_used": 23.5, "quota_monthly_start": time.Now().UTC().Format(time.RFC3339)}
			out := AccountFromServiceShallow(&service.Account{Type: typ, Extra: extra})
			require.NotNil(t, out.QuotaMonthlyLimit)
			require.Equal(t, 100.0, *out.QuotaMonthlyLimit)
			require.Equal(t, 23.5, *out.QuotaMonthlyUsed)
			require.Nil(t, out.QuotaMonthlyResetMode)
			extra["quota_monthly_start"] = time.Now().AddDate(0, -2, 0).UTC().Format(time.RFC3339)
			out = AccountFromServiceShallow(&service.Account{Type: typ, Extra: extra})
			require.Equal(t, 0.0, *out.QuotaMonthlyUsed)
			require.Equal(t, 23.5, extra["quota_monthly_used"], "mapping must not mutate persisted usage")
		})
	}
}

func TestAccountFromServiceShallowMonthlyFixedQuota(t *testing.T) {
	extra := map[string]any{"quota_monthly_limit": 100.0, "quota_monthly_used": 23.5, "quota_monthly_reset_mode": "fixed", "quota_monthly_reset_day": 31, "quota_monthly_reset_hour": 23, "quota_reset_timezone": "Asia/Shanghai"}
	service.ComputeQuotaResetAt(extra)
	out := AccountFromServiceShallow(&service.Account{Type: service.AccountTypeAPIKey, Extra: extra})
	require.Equal(t, "fixed", *out.QuotaMonthlyResetMode)
	require.Equal(t, 31, *out.QuotaMonthlyResetDay)
	require.Equal(t, 23, *out.QuotaMonthlyResetHour)
	require.Equal(t, "Asia/Shanghai", *out.QuotaResetTimezone)
	require.Equal(t, extra["quota_monthly_reset_at"], *out.QuotaMonthlyResetAt)
	require.Equal(t, 0.0, *out.QuotaMonthlyUsed)
	payload, err := json.Marshal(out)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(payload, &fields))
	require.Equal(t, 100.0, fields["quota_monthly_limit"])
	require.Equal(t, 31.0, fields["quota_monthly_reset_day"])
	require.NotContains(t, fields, "quota_notify_monthly_enabled")
}

func TestAccountFromServiceShallowMonthlyProration(t *testing.T) {
	now := time.Now()
	until := now.Add(time.Hour).UTC().Format(time.RFC3339)
	extra := map[string]any{
		"quota_monthly_limit":          100.0,
		"quota_monthly_used":           12.3,
		"quota_monthly_start":          now.Add(-time.Minute).UTC().Format(time.RFC3339),
		"quota_monthly_reset_mode":     "fixed",
		"quota_monthly_reset_day":      1,
		"quota_monthly_reset_hour":     0,
		"quota_reset_timezone":         "UTC",
		"quota_monthly_reset_at":       until,
		"quota_monthly_prorate":        true,
		"quota_monthly_prorated_limit": 50.0,
		"quota_monthly_prorated_until": until,
	}
	out := AccountFromServiceShallow(&service.Account{Type: service.AccountTypeAPIKey, Extra: extra})
	require.NotNil(t, out.QuotaMonthlyProrate)
	require.True(t, *out.QuotaMonthlyProrate)
	require.NotNil(t, out.QuotaMonthlyProratedLimit)
	require.Equal(t, 50.0, *out.QuotaMonthlyProratedLimit)
	require.NotNil(t, out.QuotaMonthlyProratedUntil)
	require.Equal(t, until, *out.QuotaMonthlyProratedUntil)
	require.NotNil(t, out.QuotaMonthlyEffectiveLimit)
	require.Equal(t, 50.0, *out.QuotaMonthlyEffectiveLimit)
	require.NotNil(t, out.QuotaMonthlyPeriodStart)

	payload, err := json.Marshal(out)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(payload, &fields))
	for _, key := range []string{
		"quota_monthly_prorate",
		"quota_monthly_prorated_limit",
		"quota_monthly_prorated_until",
		"quota_monthly_effective_limit",
		"quota_monthly_period_start",
	} {
		require.Contains(t, fields, key)
	}

	extra["quota_monthly_prorated_until"] = now.Add(-time.Hour).UTC().Format(time.RFC3339)
	out = AccountFromServiceShallow(&service.Account{Type: service.AccountTypeAPIKey, Extra: extra})
	require.Equal(t, 100.0, *out.QuotaMonthlyEffectiveLimit)
}

func TestAccountFromServiceShallowMonthlyQuotaDisabled(t *testing.T) {
	for _, extra := range []map[string]any{nil, {}, {"quota_monthly_limit": 0.0, "quota_monthly_used": 23.5}} {
		out := AccountFromServiceShallow(&service.Account{Type: service.AccountTypeAPIKey, Extra: extra})
		require.Nil(t, out.QuotaMonthlyLimit)
		require.Nil(t, out.QuotaMonthlyUsed)
	}
	out := AccountFromServiceShallow(&service.Account{Type: service.AccountTypeOAuth, Extra: map[string]any{"quota_monthly_limit": 100.0}})
	require.Nil(t, out.QuotaMonthlyLimit)
	require.Nil(t, out.QuotaMonthlyUsed)
}
