//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// alertUsageLogRepoStub 只实现今日费用批量读取（accountWindowStatsBatchReader）。
type alertUsageLogRepoStub struct {
	UsageLogRepository
	stats    map[int64]*usagestats.AccountStats
	err      error
	gotIDs   []int64
	gotStart time.Time
}

func (r *alertUsageLogRepoStub) GetAccountWindowStatsBatch(_ context.Context, ids []int64, start time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.gotIDs = ids
	r.gotStart = start
	if r.err != nil {
		return nil, r.err
	}
	return r.stats, nil
}

// alertAccountRepoStub 记录评估器取账号走的是 GetByIDs 还是 ListAllWithFilters，并返回带 extra 的整实体。
type alertAccountRepoStub struct {
	AccountRepository
	accounts      []Account
	byIDsCalls    [][]int64
	listAllCalls  []string
	listAllGroups []int64
}

func (r *alertAccountRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.byIDsCalls = append(r.byIDsCalls, ids)
	out := make([]*Account, 0, len(ids))
	for _, id := range ids {
		for i := range r.accounts {
			if r.accounts[i].ID == id {
				acc := r.accounts[i]
				out = append(out, &acc)
			}
		}
	}
	return out, nil
}

func (r *alertAccountRepoStub) ListAllWithFilters(_ context.Context, platform, _, _, _ string, groupID int64, _ string) ([]Account, error) {
	r.listAllCalls = append(r.listAllCalls, platform)
	r.listAllGroups = append(r.listAllGroups, groupID)
	out := make([]Account, 0, len(r.accounts))
	for _, acc := range r.accounts {
		if platform != "" && acc.Platform != platform {
			continue
		}
		out = append(out, acc)
	}
	return out, nil
}

func TestReadAccountWindowUsedPercent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Hour)
	past := now.Add(-time.Minute)

	tests := []struct {
		name      string
		account   *Account
		window    string
		wantValue float64
		wantOK    bool
	}{
		{
			name: "openai 5h 快照是 0-100 直接用",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"codex_5h_used_percent":  85.0,
				"codex_5h_reset_at":      future.Format(time.RFC3339),
				"codex_usage_updated_at": now.Add(-10 * time.Minute).Format(time.RFC3339),
			}},
			window: "5h", wantValue: 85, wantOK: true,
		},
		{
			name: "openai 字符串形式的百分比也能读",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"codex_7d_used_percent":  "42.5",
				"codex_7d_reset_at":      future.Format(time.RFC3339),
				"codex_usage_updated_at": now.Format(time.RFC3339),
			}},
			window: "7d", wantValue: 42.5, wantOK: true,
		},
		{
			name: "openai 没有该窗口的快照 → 无数据",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"codex_5h_used_percent": 85.0,
			}},
			window: "7d", wantOK: false,
		},
		{
			name: "openai 窗口已重置 → 新窗口记 0（告警恢复）",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"codex_5h_used_percent":  85.0,
				"codex_5h_reset_at":      past.Format(time.RFC3339),
				"codex_usage_updated_at": now.Format(time.RFC3339),
			}},
			window: "5h", wantValue: 0, wantOK: true,
		},
		{
			name: "openai 快照过期（超 2 小时没刷新）→ 无数据，不能当 0",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"codex_5h_used_percent":  85.0,
				"codex_5h_reset_at":      future.Format(time.RFC3339),
				"codex_usage_updated_at": now.Add(-3 * time.Hour).Format(time.RFC3339),
			}},
			window: "5h", wantOK: false,
		},
		{
			name: "anthropic 存的是 0-1 小数，必须换算成百分比",
			account: &Account{Platform: PlatformAnthropic, SessionWindowEnd: &future, Extra: map[string]any{
				"session_window_utilization": 0.85,
			}},
			window: "5h", wantValue: 85, wantOK: true,
		},
		{
			name: "anthropic 7d 利用率 + unix 重置时间",
			account: &Account{Platform: PlatformAnthropic, Extra: map[string]any{
				"passive_usage_7d_utilization": 0.42,
				"passive_usage_7d_reset":       float64(future.Unix()),
			}},
			window: "7d", wantValue: 42, wantOK: true,
		},
		{
			name: "anthropic 会话窗口已结束 → 记 0",
			account: &Account{Platform: PlatformAnthropic, SessionWindowEnd: &past, Extra: map[string]any{
				"session_window_utilization": 0.85,
			}},
			window: "5h", wantValue: 0, wantOK: true,
		},
		{
			name:    "anthropic 没有快照 → 无数据",
			account: &Account{Platform: PlatformAnthropic, Extra: map[string]any{}},
			window:  "5h", wantOK: false,
		},
		{
			name: "kimi 5h 用 kimi_5h_used_percent",
			account: &Account{Platform: PlatformKimi, Extra: map[string]any{
				"kimi_5h_used_percent": 60.0,
				"kimi_5h_reset_at":     future.Format(time.RFC3339),
			}},
			window: "5h", wantValue: 60, wantOK: true,
		},
		{
			name: "kimi 7d 映射到 weekly 键",
			account: &Account{Platform: PlatformKimi, Extra: map[string]any{
				"kimi_weekly_used_percent": 30.0,
				"kimi_weekly_reset_at":     future.Format(time.RFC3339),
			}},
			window: "7d", wantValue: 30, wantOK: true,
		},
		{
			name: "grok 滚动配额窗口挂在 5h 下",
			account: &Account{Platform: PlatformGrok, Extra: map[string]any{
				"grok_sched_utilization": 77.0,
				"grok_sched_reset_at":    future.Format(time.RFC3339),
			}},
			window: "5h", wantValue: 77, wantOK: true,
		},
		{
			name:    "gemini 没有窗口快照 → 无数据",
			account: &Account{Platform: PlatformGemini, Extra: map[string]any{"codex_5h_used_percent": 90.0}},
			window:  "5h", wantOK: false,
		},
		{
			name:    "非法窗口 → 无数据",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"codex_5h_used_percent": 90.0}},
			window:  "1h", wantOK: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := readAccountWindowUsedPercent(tt.account, tt.window, now)
			require.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.InDelta(t, tt.wantValue, got, 0.0001)
			}
		})
	}
}

func TestReadAccountQuotaUsedPercent(t *testing.T) {
	t.Parallel()

	daily := &Account{Extra: map[string]any{"quota_daily_limit": 100.0, "quota_daily_used": 40.0}}
	got, ok := readAccountQuotaUsedPercent(daily, "daily")
	require.True(t, ok)
	require.InDelta(t, 40, got, 0.0001)

	weekly := &Account{Extra: map[string]any{"quota_weekly_limit": "200", "quota_weekly_used": "150"}}
	got, ok = readAccountQuotaUsedPercent(weekly, "weekly")
	require.True(t, ok)
	require.InDelta(t, 75, got, 0.0001)

	total := &Account{Extra: map[string]any{"quota_limit": 50.0, "quota_used": 60.0}}
	got, ok = readAccountQuotaUsedPercent(total, "total")
	require.True(t, ok)
	require.InDelta(t, 120, got, 0.0001, "超额也如实返回")

	_, ok = readAccountQuotaUsedPercent(&Account{Extra: map[string]any{"quota_daily_used": 40.0}}, "daily")
	require.False(t, ok, "没有 limit → 无数据")
	_, ok = readAccountQuotaUsedPercent(&Account{Extra: map[string]any{"quota_daily_limit": 0.0, "quota_daily_used": 40.0}}, "daily")
	require.False(t, ok, "limit ≤ 0 → 无数据")
	_, ok = readAccountQuotaUsedPercent(daily, "monthly")
	require.False(t, ok, "非法维度 → 无数据")
}

func TestReadAccountBalance(t *testing.T) {
	t.Parallel()

	kimi := &Account{Platform: PlatformKimi, Extra: map[string]any{"kimi_balance": 3.5, "kimi_balance_currency": "CNY"}}
	got, currency, ok := readAccountBalance(kimi)
	require.True(t, ok)
	require.InDelta(t, 3.5, got, 0.0001)
	require.Equal(t, "CNY", currency)

	deepseek := &Account{Platform: PlatformDeepseek, Extra: map[string]any{"deepseek_balance": "12.25"}}
	got, currency, ok = readAccountBalance(deepseek)
	require.True(t, ok)
	require.InDelta(t, 12.25, got, 0.0001)
	require.Equal(t, "", currency)

	_, _, ok = readAccountBalance(&Account{Platform: PlatformKimi, Extra: map[string]any{"kimi_5h_used_percent": 10.0}})
	require.False(t, ok, "没有余额键（coding 套餐）→ 无数据")
	_, _, ok = readAccountBalance(&Account{Platform: PlatformOpenAI, Extra: map[string]any{"openai_balance": 9.0}})
	require.False(t, ok, "非 kimi / deepseek → 无数据")
}

func TestReadAccountTodayCosts(t *testing.T) {
	t.Parallel()

	accounts := []*Account{{ID: 1}, {ID: 2}}

	repo := &alertUsageLogRepoStub{stats: map[int64]*usagestats.AccountStats{1: {Cost: 1.5}}}
	svc := &OpsAlertEvaluatorService{usageLogRepo: repo}
	costs, ok := svc.readAccountTodayCosts(context.Background(), accounts)
	require.True(t, ok)
	require.InDelta(t, 1.5, costs[1], 0.0001)
	require.InDelta(t, 0, costs[2], 0.0001, "今天没请求的账号费用是 0，不是无数据")
	require.Equal(t, []int64{1, 2}, repo.gotIDs)
	require.True(t, repo.gotStart.Equal(timezone.Today()), "起点应是本地时区的今天零点")

	failing := &OpsAlertEvaluatorService{usageLogRepo: &alertUsageLogRepoStub{err: errors.New("db down")}}
	_, ok = failing.readAccountTodayCosts(context.Background(), accounts)
	require.False(t, ok, "批量查询失败 → 整体无数据")

	bare := &OpsAlertEvaluatorService{}
	_, ok = bare.readAccountTodayCosts(context.Background(), accounts)
	require.False(t, ok, "没有用量日志仓储 → 无数据")
}

func TestParseOpsAlertAccountIDs(t *testing.T) {
	t.Parallel()

	require.Nil(t, ParseOpsAlertAccountIDs(nil))
	require.Nil(t, ParseOpsAlertAccountIDs("x"))
	require.Equal(t, []int64{3, 4}, ParseOpsAlertAccountIDs([]any{float64(3), "4", float64(3), float64(-1), "bad"}))
	require.Equal(t, []int64{5}, ParseOpsAlertAccountIDs([]int64{5, 0}))
	require.Equal(t, []int64{7, 8}, ParseOpsAlertAccountIDs([]float64{7, 8}))
}

func TestCollectAccountMetricSamples_UsesFiltersAndSkipsNoData(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	future := now.Add(time.Hour)
	var gotPlatform string
	var gotGroupID *int64
	var gotIDs []int64
	opsService := &OpsService{
		listAccountsForAlerts: func(_ context.Context, platform string, groupID *int64, accountIDs []int64) ([]*Account, error) {
			gotPlatform, gotGroupID, gotIDs = platform, groupID, accountIDs
			return []*Account{
				{ID: 2, Name: "b", Platform: PlatformOpenAI, Extra: map[string]any{
					"codex_5h_used_percent": 90.0, "codex_5h_reset_at": future.Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339),
				}},
				{ID: 3, Name: "no-data", Platform: PlatformOpenAI, Extra: map[string]any{}},
				{ID: 1, Name: "a", Platform: PlatformOpenAI, Extra: map[string]any{
					"codex_5h_used_percent": 10.0, "codex_5h_reset_at": future.Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339),
				}},
			}, nil
		},
	}
	svc := &OpsAlertEvaluatorService{opsService: opsService}
	rule := &OpsAlertRule{
		MetricType: OpsAlertMetricAccountWindowUsedPercent,
		Filters:    map[string]any{"platform": "openai", "group_id": float64(3), "window": "5h", "account_ids": []any{float64(2), float64(1), float64(3)}},
	}

	samples, err := svc.collectAccountMetricSamples(context.Background(), rule, now)
	require.NoError(t, err)
	require.Equal(t, "openai", gotPlatform)
	require.NotNil(t, gotGroupID)
	require.Equal(t, int64(3), *gotGroupID)
	require.Equal(t, []int64{2, 1, 3}, gotIDs)

	require.Len(t, samples, 2, "无数据的账号被跳过")
	require.Equal(t, int64(1), samples[0].AccountID, "按账号 ID 升序")
	require.InDelta(t, 10, samples[0].Value, 0.0001)
	require.Equal(t, int64(2), samples[1].AccountID)
	require.InDelta(t, 90, samples[1].Value, 0.0001)
	require.Equal(t, "b", samples[1].AccountName)
	require.Equal(t, "openai", samples[1].Platform)
}

func TestCollectAccountMetricSamples_BalanceUsesProviderAsPlatform(t *testing.T) {
	t.Parallel()

	var gotPlatform string
	opsService := &OpsService{
		listAccountsForAlerts: func(_ context.Context, platform string, _ *int64, _ []int64) ([]*Account, error) {
			gotPlatform = platform
			return []*Account{{ID: 9, Name: "k", Platform: PlatformKimi, Extra: map[string]any{"kimi_balance": 2.0, "kimi_balance_currency": "CNY"}}}, nil
		},
	}
	svc := &OpsAlertEvaluatorService{opsService: opsService}
	rule := &OpsAlertRule{MetricType: OpsAlertMetricAccountBalance, Filters: map[string]any{"provider": "kimi"}}

	samples, err := svc.collectAccountMetricSamples(context.Background(), rule, time.Now())
	require.NoError(t, err)
	require.Equal(t, "kimi", gotPlatform)
	require.Len(t, samples, 1)
	require.InDelta(t, 2, samples[0].Value, 0.0001)
	require.Equal(t, "CNY", samples[0].Currency)
}

func TestOpsService_ListAccountsForAlerts_UsesFullEntityReads(t *testing.T) {
	t.Parallel()

	repo := &alertAccountRepoStub{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, GroupIDs: []int64{3}, Extra: map[string]any{"codex_5h_used_percent": 85.0}},
		{ID: 2, Platform: PlatformAnthropic, GroupIDs: []int64{3}, Extra: map[string]any{"session_window_utilization": 0.5}},
		{ID: 4, Platform: PlatformOpenAI, GroupIDs: []int64{7}, Extra: map[string]any{"codex_5h_used_percent": 15.0}},
	}}
	svc := &OpsService{accountRepo: repo}
	groupID := int64(3)

	// 指定 account_ids：走 GetByIDs，再按 platform / group 过滤。
	got, err := svc.ListAccountsForAlerts(context.Background(), "openai", &groupID, []int64{1, 2, 4})
	require.NoError(t, err)
	require.Len(t, repo.byIDsCalls, 1)
	require.Empty(t, repo.listAllCalls)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, 85.0, got[0].Extra["codex_5h_used_percent"], "取回的账号必须带 extra 快照")

	// 不指定 account_ids：走 ListAllWithFilters，把 platform / group 透传给仓储。
	got, err = svc.ListAccountsForAlerts(context.Background(), "openai", &groupID, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"openai"}, repo.listAllCalls)
	require.Equal(t, []int64{3}, repo.listAllGroups)
	require.Len(t, got, 2)
	for _, acc := range got {
		require.NotEmpty(t, acc.Extra, "取回的账号必须带 extra 快照")
	}
}

func TestOpsAlertAccountMetricLabelsAndUnits(t *testing.T) {
	t.Parallel()

	require.Equal(t, "账号 5 小时窗口用量", opsAlertAccountMetricLabel(OpsAlertMetricAccountWindowUsedPercent, opsAlertAccountFilters{Window: "5h"}))
	require.Equal(t, "账号 7 天窗口用量", opsAlertAccountMetricLabel(OpsAlertMetricAccountWindowUsedPercent, opsAlertAccountFilters{Window: "7d"}))
	require.Equal(t, "账号周额度用量", opsAlertAccountMetricLabel(OpsAlertMetricAccountQuotaUsedPercent, opsAlertAccountFilters{Dimension: "weekly"}))
	require.Equal(t, "账号余额", opsAlertAccountMetricLabel(OpsAlertMetricAccountBalance, opsAlertAccountFilters{}))
	require.Equal(t, "账号今日费用", opsAlertAccountMetricLabel(OpsAlertMetricAccountTodayCost, opsAlertAccountFilters{}))
	require.Equal(t, "cpu_usage_percent", opsAlertAccountMetricLabel("cpu_usage_percent", opsAlertAccountFilters{}))

	require.Equal(t, "%", opsAlertAccountMetricUnit(OpsAlertMetricAccountWindowUsedPercent, ""))
	require.Equal(t, " CNY", opsAlertAccountMetricUnit(OpsAlertMetricAccountBalance, "CNY"))
	require.Equal(t, "", opsAlertAccountMetricUnit(OpsAlertMetricAccountBalance, ""))
	require.Equal(t, " USD", opsAlertAccountMetricUnit(OpsAlertMetricAccountTodayCost, ""))

	rule := &OpsAlertRule{MetricType: OpsAlertMetricAccountWindowUsedPercent, Operator: ">=", Threshold: 80}
	sample := accountMetricSample{AccountID: 1, AccountName: "codex-01", Platform: "openai", Value: 85}
	filters := opsAlertAccountFilters{Window: "5h", GroupID: func() *int64 { v := int64(3); return &v }()}
	require.Equal(t, "账号 5 小时窗口用量：账号 codex-01（openai）当前 85%，阈值 >= 80%", buildOpsAlertAccountDescription(rule, sample, filters))
	dims := buildOpsAlertAccountDimensions(sample, filters)
	require.Equal(t, int64(1), dims["account_id"])
	require.Equal(t, "codex-01", dims["account_name"])
	require.Equal(t, "openai", dims["platform"])
	require.Equal(t, int64(3), dims["group_id"])
	require.Equal(t, "5h", dims["window"])
	require.Equal(t, "账号：codex-01（openai）", formatOpsAlertAccountLine(sample))
}
