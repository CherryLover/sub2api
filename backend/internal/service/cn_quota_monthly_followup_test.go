package service

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cnQuotaSnapshotRepo struct {
	AccountRepository
	account *Account
	updates map[string]any
	calls   int
}

func (r *cnQuotaSnapshotRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *cnQuotaSnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.calls++
	r.updates = updates
	return nil
}

type brokenCNQuotaBody struct{ data string }

func (b *brokenCNQuotaBody) Read(p []byte) (int, error) {
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, io.ErrUnexpectedEOF
}
func (b *brokenCNQuotaBody) Close() error { return nil }
func TestKimiMonthlyQuotaProbePreservesInvalidSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		broken     bool
	}{
		{"empty object", `{}`, false}, {"invalid json", `{"usage":`, false},
		{"invalid ratio beside valid window", `{"usages":{"limit_5h":{"used_ratio":0.2},"limit_month_total":{"used_ratio":"NaN"}}}`, false},
		{"malformed legacy beside valid weekly", `{"limits":[{"detail":{}}],"usage":{"limit":10,"remaining":5}}`, false},
		{"valid bytes with read failure", `{"usage":{"limit":10,"remaining":5}}`, true},
		{"oversized reply", `{"usage":{"limit":10,"remaining":5}}` + strings.Repeat(" ", cnQuotaMaxBodyBytes), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := codingAccount(PlatformKimi)
			account.Credentials["base_url"] = "https://api.kimi.com/coding/v1"
			account.Extra = map[string]any{"kimi_weekly_used_percent": 100, "kimi_usage_updated_at": "old"}
			repo := &cnQuotaSnapshotRepo{account: account}
			body := io.NopCloser(strings.NewReader(tc.body))
			if tc.broken {
				body = &brokenCNQuotaBody{data: tc.body}
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}}
			result, err := NewCNProviderQuotaService(repo, nil, upstream, cnProbeAllowlistConfig("api.kimi.com")).QueryUsageForAccount(t.Context(), account)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.Success)
			require.Zero(t, repo.calls)
			require.Equal(t, "old", account.Extra["kimi_usage_updated_at"])
		})
	}
}
func TestKimiMonthlyQuotaProbeReplacesAbsentWeeklyWindow(t *testing.T) {
	reset := time.Now().Add(20 * time.Hour).UTC().Format(time.RFC3339)
	account := codingAccount(PlatformKimi)
	account.Credentials["base_url"] = "https://api.kimi.com/coding/v1"
	repo := &cnQuotaSnapshotRepo{account: account}
	body := fmt.Sprintf(`{"usages":{"limit_5h":{"used_ratio":0.2,"reset_time":%q},"limit_month_total":{"used_ratio":0.7,"reset_time":%q},"limit_month_code":{"used_ratio":0.95,"reset_time":%q}}}`, reset, reset, reset)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}}
	result, err := NewCNProviderQuotaService(repo, nil, upstream, cnProbeAllowlistConfig("api.kimi.com")).QueryUsageForAccount(t.Context(), account)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.Persisted)
	require.Equal(t, float64(95), repo.updates["kimi_monthly_used_percent"])
	require.Contains(t, repo.updates, "kimi_weekly_used_percent")
	require.Nil(t, repo.updates["kimi_weekly_used_percent"])
	require.Contains(t, repo.updates, "kimi_weekly_reset_at")
	require.Nil(t, repo.updates["kimi_weekly_reset_at"])
	candidate := cnThresholdCandidate(repo.updates, PlatformKimi, "monthly")
	require.NotNil(t, candidate)
	require.True(t, candidateMatchesThreshold(candidate, 90, time.Now()))
}
func TestKimiMonthlyQuotaParserChoosesConservativeCombinedWindow(t *testing.T) {
	body := []byte(`{"usages":{"limit_5h":{"used_ratio":0},"limit_month_total":{"used_ratio":1,"reset_time":"2026-12-26T00:00:00Z"},"limit_month_code":{"used_ratio":0.3,"reset_time":"2026-12-20T00:00:00Z"}}}`)
	tiers := parseKimiUsageTiers(body)
	require.Len(t, tiers, 2)
	require.Equal(t, float64(0), tiers[0].UsedPercent)
	require.Equal(t, "monthly", tiers[1].Window)
	require.Equal(t, float64(100), tiers[1].UsedPercent)
	require.Equal(t, "2026-12-26T00:00:00Z", tiers[1].ResetAt)
}
