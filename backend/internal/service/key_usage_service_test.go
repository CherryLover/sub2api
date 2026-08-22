package service

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"

	"github.com/stretchr/testify/require"
)

// --- 测试替身 -------------------------------------------------------------

type fakeKeyUsageAPIKeys struct {
	byKey map[string]*APIKey
	byID  map[int64]*APIKey
}

func (f *fakeKeyUsageAPIKeys) GetByKey(_ context.Context, key string) (*APIKey, error) {
	if apiKey, ok := f.byKey[key]; ok {
		return apiKey, nil
	}
	return nil, ErrAPIKeyNotFound
}

func (f *fakeKeyUsageAPIKeys) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if apiKey, ok := f.byID[id]; ok {
		return apiKey, nil
	}
	return nil, ErrAPIKeyNotFound
}

type fakeKeyUsageModelStats struct {
	byKey map[int64][]usagestats.ModelStat
	err   error
}

func (f *fakeKeyUsageModelStats) GetAPIKeyModelStats(_ context.Context, apiKeyID int64, _, _ time.Time) ([]usagestats.ModelStat, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byKey[apiKeyID], nil
}

type fakeKeyUsageRanking struct {
	mu        sync.Mutex
	rows      []usagestats.APIKeyUsageAggregate
	names     map[int64]string
	aggCalls  int
	siteCalls int
	err       error
}

func (f *fakeKeyUsageRanking) GetAPIKeyUsageAggregates(_ context.Context, _, _ time.Time, userID int64, metric string) ([]usagestats.APIKeyUsageAggregate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aggCalls++
	if userID == 0 {
		f.siteCalls++
	}
	if f.err != nil {
		return nil, f.err
	}
	return sortedAggregatesForMetric(f.rows, metric), nil
}

func (f *fakeKeyUsageRanking) GetAPIKeyNamesByIDs(_ context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		if name, ok := f.names[id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

// sortedAggregatesForMetric 模拟 SQL 的 ORDER BY <metric> DESC, api_key_id ASC。
func sortedAggregatesForMetric(rows []usagestats.APIKeyUsageAggregate, metric string) []usagestats.APIKeyUsageAggregate {
	out := make([]usagestats.APIKeyUsageAggregate, len(rows))
	copy(out, rows)
	values := keyUsageMetricValues(out, metric)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if values[j] > values[j-1] || (values[j] == values[j-1] && out[j].APIKeyID < out[j-1].APIKeyID) {
				out[j], out[j-1] = out[j-1], out[j]
				values[j], values[j-1] = values[j-1], values[j]
				continue
			}
			break
		}
	}
	return out
}

func newTestKeyUsageService(t *testing.T, keys *fakeKeyUsageAPIKeys, stats *fakeKeyUsageModelStats, ranking KeyUsageRankingRepository, cacheTTL time.Duration) *KeyUsageService {
	t.Helper()
	tokens := newTestKeyUsageTokenService(t)
	return NewKeyUsageService(keys, stats, ranking, tokens, cacheTTL)
}

func activeTestAPIKey(id int64, userID int64, key, name string) *APIKey {
	return &APIKey{ID: id, UserID: userID, Key: key, Name: name, Status: StatusAPIKeyActive, CreatedAt: time.Now()}
}

// --- 鉴权 -----------------------------------------------------------------

// 无效 key 与被禁用 key 必须返回完全相同的错误，否则这个免登录端点就成了 key 探测器。
func TestKeyUsageResolveRawKeyHidesKeyExistence(t *testing.T) {
	disabled := activeTestAPIKey(2, 1, "sk-disabled", "disabled")
	disabled.Status = StatusAPIKeyDisabled
	keys := &fakeKeyUsageAPIKeys{
		byKey: map[string]*APIKey{"sk-disabled": disabled},
		byID:  map[int64]*APIKey{2: disabled},
	}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)

	_, missingErr := svc.ResolveRawKey(context.Background(), "sk-does-not-exist")
	_, disabledErr := svc.ResolveRawKey(context.Background(), "sk-disabled")
	_, emptyErr := svc.ResolveRawKey(context.Background(), "   ")

	require.ErrorIs(t, missingErr, ErrKeyUsageUnauthorized)
	require.ErrorIs(t, disabledErr, ErrKeyUsageUnauthorized)
	require.ErrorIs(t, emptyErr, ErrKeyUsageUnauthorized)
	require.Equal(t, missingErr.Error(), disabledErr.Error())
}

// 额度耗尽/已过期的 key 仍然可以看自己的用量（与 /v1/usage 的 isValid 口径一致）。
func TestKeyUsageResolveRawKeyAllowsQuotaExhaustedAndExpired(t *testing.T) {
	exhausted := activeTestAPIKey(3, 1, "sk-exhausted", "k3")
	exhausted.Status = StatusAPIKeyQuotaExhausted
	expired := activeTestAPIKey(4, 1, "sk-expired", "k4")
	expired.Status = StatusAPIKeyExpired
	keys := &fakeKeyUsageAPIKeys{byKey: map[string]*APIKey{"sk-exhausted": exhausted, "sk-expired": expired}}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)

	got, err := svc.ResolveRawKey(context.Background(), "sk-exhausted")
	require.NoError(t, err)
	require.Equal(t, int64(3), got.ID)

	got, err = svc.ResolveRawKey(context.Background(), "sk-expired")
	require.NoError(t, err)
	require.Equal(t, int64(4), got.ID)
}

func TestKeyUsageTokenLifecycle(t *testing.T) {
	apiKey := activeTestAPIKey(11, 5, "sk-live", "my key")
	keys := &fakeKeyUsageAPIKeys{
		byKey: map[string]*APIKey{"sk-live": apiKey},
		byID:  map[int64]*APIKey{11: apiKey},
	}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)
	ctx := context.Background()

	token, expiresAt, err := svc.IssueToken(ctx, "sk-live")
	require.NoError(t, err)
	require.True(t, expiresAt.After(timezone.Now()))

	resolved, err := svc.ResolveToken(ctx, token)
	require.NoError(t, err)
	require.Equal(t, int64(11), resolved.ID)

	t.Run("令牌不是网关凭证", func(t *testing.T) {
		require.NotContains(t, token, "sk-live")
		_, err := svc.ResolveRawKey(ctx, token)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("key 被禁用后立即失效", func(t *testing.T) {
		apiKey.Status = StatusAPIKeyDisabled
		defer func() { apiKey.Status = StatusAPIKeyActive }()
		_, err := svc.ResolveToken(ctx, token)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("key 被删除后立即失效", func(t *testing.T) {
		delete(keys.byID, 11)
		defer func() { keys.byID[11] = apiKey }()
		_, err := svc.ResolveToken(ctx, token)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("key 值轮换后旧令牌失效", func(t *testing.T) {
		original := apiKey.Key
		apiKey.Key = "sk-rotated"
		defer func() { apiKey.Key = original }()
		_, err := svc.ResolveToken(ctx, token)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("被禁用的 key 换不到令牌", func(t *testing.T) {
		apiKey.Status = StatusAPIKeyDisabled
		defer func() { apiKey.Status = StatusAPIKeyActive }()
		_, _, err := svc.IssueToken(ctx, "sk-live")
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})
}

// --- 窗口口径 -------------------------------------------------------------

// 窗口边界必须与按天曲线同源：起点是自然日 00:00，终点是明天 00:00。
func TestKeyUsageWindowRangesUseCalendarDayBoundaries(t *testing.T) {
	now := timezone.Now()
	ranges := KeyUsageWindowRanges(now)

	today := ranges[KeyUsageWindowToday]
	require.Equal(t, timezone.StartOfDay(now), today[0])
	require.Equal(t, timezone.StartOfDay(now).AddDate(0, 0, 1), today[1])

	require.Equal(t, timezone.StartOfDay(now.AddDate(0, 0, -6)), ranges[KeyUsageWindowLast7d][0])
	require.Equal(t, today[1], ranges[KeyUsageWindowLast7d][1])
	require.Equal(t, timezone.StartOfDay(now.AddDate(0, 0, -29)), ranges[KeyUsageWindowLast30d][0])
	require.Equal(t, today[1], ranges[KeyUsageWindowLast30d][1])
}

// 跨月/跨年时窗口起点仍然落在自然日边界上。
func TestKeyUsageWindowRangesAcrossMonthBoundary(t *testing.T) {
	loc := timezone.Location()
	now := time.Date(2026, time.March, 2, 3, 14, 15, 0, loc)
	ranges := KeyUsageWindowRanges(now)

	require.Equal(t, time.Date(2026, time.March, 2, 0, 0, 0, 0, loc), ranges[KeyUsageWindowToday][0])
	require.Equal(t, time.Date(2026, time.March, 3, 0, 0, 0, 0, loc), ranges[KeyUsageWindowToday][1])
	require.Equal(t, time.Date(2026, time.February, 24, 0, 0, 0, 0, loc), ranges[KeyUsageWindowLast7d][0])
	require.Equal(t, time.Date(2026, time.February, 1, 0, 0, 0, 0, loc), ranges[KeyUsageWindowLast30d][0])
}

func TestKeyUsageWindowStatsSumModelRows(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	stats := &fakeKeyUsageModelStats{byKey: map[int64][]usagestats.ModelStat{
		1: {
			{Model: "claude-opus-5", Requests: 10, TotalTokens: 5000, ActualCost: 0.5},
			{Model: "claude-haiku-5", Requests: 2, TotalTokens: 300, ActualCost: 0.1},
		},
	}}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, stats, nil, time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, "")
	require.Equal(t, int64(12), report.Windows.Today.Requests)
	require.Equal(t, int64(5300), report.Windows.Today.Tokens)
	require.InDelta(t, 0.6, report.Windows.Today.CostUSD, 1e-9)
	require.Len(t, report.Windows.Today.Models, 2)
	require.Equal(t, "claude-opus-5", report.Windows.Today.Models[0].Model)
	// 三个窗口结构一致
	require.Equal(t, report.Windows.Today, report.Windows.Last30d)
}

func TestKeyUsageEmptyDataReturnsZeroValues(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	ranking := &fakeKeyUsageRanking{}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, &fakeKeyUsageModelStats{}, ranking, time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, "")
	require.Equal(t, int64(0), report.Windows.Today.Requests)
	require.NotNil(t, report.Windows.Today.Models)
	require.Empty(t, report.Windows.Today.Models)

	for _, scope := range []KeyUsageRankingScope{report.Rankings.Account, report.Rankings.Site} {
		require.NotNil(t, scope.Today.Top)
		require.Empty(t, scope.Today.Top)
		// 自己没有用量时也要出现在总数里，避免 self_rank > total_keys
		require.Equal(t, 1, scope.Today.TotalKeys)
		require.Equal(t, 1, scope.Today.SelfRank)
		require.True(t, scope.Today.Self.IsSelf)
		require.Equal(t, "self", scope.Today.Self.KeyName)
	}
}

// --- 排名 -----------------------------------------------------------------

func rankingFixture() *fakeKeyUsageRanking {
	rows := []usagestats.APIKeyUsageAggregate{
		{APIKeyID: 1, Requests: 5, Tokens: 500, Cost: 5},   // self
		{APIKeyID: 2, Requests: 30, Tokens: 100, Cost: 30}, // 榜首
		{APIKeyID: 3, Requests: 10, Tokens: 900, Cost: 10},
		{APIKeyID: 4, Requests: 10, Tokens: 800, Cost: 10}, // 与 3 在 cost/requests 上并列
	}
	names := map[int64]string{1: "self", 2: "top", 3: "third", 4: "fourth"}
	return &fakeKeyUsageRanking{rows: rows, names: names}
}

func selfWithUsage(requests int64, tokens int64, cost float64) *fakeKeyUsageModelStats {
	return &fakeKeyUsageModelStats{byKey: map[int64][]usagestats.ModelStat{
		1: {{Model: "claude-opus-5", Requests: requests, TotalTokens: tokens, ActualCost: cost}},
	}}
}

// 并列策略：标准竞赛排名（1224）——名次 = 严格大于自己的 Key 数 + 1。
func TestKeyUsageRankingTiesUseCompetitionRanking(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), rankingFixture(), time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost)
	account := report.Rankings.Account.Today

	require.Equal(t, 4, account.TotalKeys)
	// cost: 30(id2) > 10(id3)=10(id4) > 5(id1)
	require.Equal(t, []int{1, 2, 2, 4}, ranksOf(account.Top))
	require.Equal(t, []string{"top", "third", "fourth", "self"}, namesOf(account.Top))
	require.Equal(t, 4, account.SelfRank)
	require.Equal(t, 4, account.Self.Rank)
	require.True(t, account.Self.IsSelf)
	require.Equal(t, []bool{false, false, false, true}, selfFlagsOf(account.Top))
	// self 即使已经出现在 top 里也照样单独返回一份
	require.Equal(t, int64(5), account.Self.Requests)
	require.InDelta(t, 5, account.Self.CostUSD, 1e-9)
}

func TestKeyUsageRankingMetricSwitching(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")

	cases := []struct {
		metric    string
		wantNames []string
		wantSelf  int
	}{
		// tokens: 900(id3) > 800(id4) > 500(id1) > 100(id2)
		{usagestats.KeyRankingMetricTokens, []string{"third", "fourth", "self", "top"}, 3},
		// requests: 30(id2) > 10(id3)=10(id4) > 5(id1)
		{usagestats.KeyRankingMetricRequests, []string{"top", "third", "fourth", "self"}, 4},
		// cost 同 requests 分布
		{usagestats.KeyRankingMetricCost, []string{"top", "third", "fourth", "self"}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.metric, func(t *testing.T) {
			svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), rankingFixture(), time.Minute)
			report := svc.BuildReport(context.Background(), apiKey, tc.metric)
			require.Equal(t, tc.metric, report.Metric)
			require.Equal(t, tc.wantNames, namesOf(report.Rankings.Site.Today.Top))
			require.Equal(t, tc.wantSelf, report.Rankings.Site.Today.SelfRank)
		})
	}
}

func TestKeyUsageRankingFallsBackToCostForUnknownMetric(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), rankingFixture(), time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, "DROP TABLE usage_logs")
	require.Equal(t, usagestats.KeyRankingMetricCost, report.Metric)
}

func TestKeyUsageRankingTruncatesTopToTen(t *testing.T) {
	rows := make([]usagestats.APIKeyUsageAggregate, 0, 25)
	names := make(map[int64]string, 25)
	for i := 1; i <= 25; i++ {
		rows = append(rows, usagestats.APIKeyUsageAggregate{APIKeyID: int64(i), Requests: int64(i), Tokens: int64(i * 10), Cost: float64(i)})
		names[int64(i)] = "key-" + strconv.Itoa(i)
	}
	ranking := &fakeKeyUsageRanking{rows: rows, names: names}
	apiKey := activeTestAPIKey(1, 1, "sk-1", "key-1")
	stats := &fakeKeyUsageModelStats{byKey: map[int64][]usagestats.ModelStat{
		1: {{Model: "m", Requests: 1, TotalTokens: 10, ActualCost: 1}},
	}}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, stats, ranking, time.Minute)

	site := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost).Rankings.Site.Today
	require.Len(t, site.Top, KeyUsageTopN)
	require.Equal(t, "key-25", site.Top[0].KeyName)
	require.Equal(t, 25, site.TotalKeys)
	// 自己是最低的一个：名次 25，且不在 top 里，但 self 仍然返回
	require.Equal(t, 25, site.SelfRank)
	require.Equal(t, "key-1", site.Self.KeyName)
	require.True(t, site.Self.IsSelf)
	for _, entry := range site.Top {
		require.False(t, entry.IsSelf)
	}
}

// --- 全站缓存 -------------------------------------------------------------

func TestKeyUsageSiteRankingCacheIsSharedAndMetricScoped(t *testing.T) {
	ranking := rankingFixture()
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKeyA := activeTestAPIKey(1, 1, "sk-1", "self")
	apiKeyB := activeTestAPIKey(9, 2, "sk-9", "other")
	ctx := context.Background()

	svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricCost)
	require.Equal(t, 3, ranking.siteCalls, "首次请求：三个窗口各查一次全站榜")

	// 换一把 key（不同账户）也应命中同一份全站缓存
	svc.BuildReport(ctx, apiKeyB, usagestats.KeyRankingMetricCost)
	require.Equal(t, 3, ranking.siteCalls, "全站榜与具体 Key 无关，必须复用缓存")

	// 换 metric 必须重新聚合，不能串味
	svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricTokens)
	require.Equal(t, 6, ranking.siteCalls)

	tokensTop := svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricTokens).Rankings.Site.Today
	costTop := svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricCost).Rankings.Site.Today
	require.Equal(t, 6, ranking.siteCalls, "两个 metric 都已缓存")
	require.Equal(t, "third", tokensTop.Top[0].KeyName)
	require.Equal(t, "top", costTop.Top[0].KeyName)
}

func TestKeyUsageSiteRankingCacheExpires(t *testing.T) {
	ranking := rankingFixture()
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, 20*time.Millisecond)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	ctx := context.Background()

	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost)
	require.Equal(t, 3, ranking.siteCalls)

	time.Sleep(40 * time.Millisecond)
	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost)
	require.Equal(t, 6, ranking.siteCalls, "TTL 过期后必须重新聚合")
}

// 账户内榜每次都实时查（数据量小），不吃全站缓存。
func TestKeyUsageAccountRankingIsNotCached(t *testing.T) {
	ranking := rankingFixture()
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	ctx := context.Background()

	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost)
	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost)

	ranking.mu.Lock()
	defer ranking.mu.Unlock()
	require.Equal(t, 3, ranking.siteCalls)
	require.Equal(t, 9, ranking.aggCalls, "账户榜 2 次 × 3 窗口 + 全站榜 3 次")
}

// 排名查询失败时页面其余部分照常渲染，排名板块退化成零值。
func TestKeyUsageRankingDegradesOnRepositoryError(t *testing.T) {
	ranking := &fakeKeyUsageRanking{err: context.DeadlineExceeded}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")

	report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost)
	require.Equal(t, int64(5), report.Windows.Today.Requests)
	require.Empty(t, report.Rankings.Site.Today.Top)
	require.Equal(t, 1, report.Rankings.Site.Today.SelfRank)
	require.True(t, report.Rankings.Account.Today.Self.IsSelf)
}

func ranksOf(entries []KeyUsageRankEntry) []int {
	out := make([]int, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Rank)
	}
	return out
}

func namesOf(entries []KeyUsageRankEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.KeyName)
	}
	return out
}

func selfFlagsOf(entries []KeyUsageRankEntry) []bool {
	out := make([]bool, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.IsSelf)
	}
	return out
}

// 账号被封禁时，即使 Key 本身还是 active 也不允许再查用量（纵深防御）。
func TestKeyUsageRejectsInactiveOwner(t *testing.T) {
	apiKey := activeTestAPIKey(21, 9, "sk-banned-owner", "k")
	apiKey.User = &User{ID: 9, Status: "disabled"}
	keys := &fakeKeyUsageAPIKeys{
		byKey: map[string]*APIKey{"sk-banned-owner": apiKey},
		byID:  map[int64]*APIKey{21: apiKey},
	}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)
	ctx := context.Background()

	_, err := svc.ResolveRawKey(ctx, "sk-banned-owner")
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)

	// 令牌是在账号正常时签发的，封禁后必须立刻失效
	apiKey.User.Status = StatusActive
	token, _, err := svc.IssueToken(ctx, "sk-banned-owner")
	require.NoError(t, err)
	apiKey.User.Status = "disabled"
	_, err = svc.ResolveToken(ctx, token)
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
}

// 排名退化成零值时 total_keys 不能小于 self_rank（前端会用它渲染"第 N / 共 M"）。
func TestKeyUsageTotalKeysNeverBelowSelfRank(t *testing.T) {
	ranking := &fakeKeyUsageRanking{err: context.DeadlineExceeded}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")

	report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost)
	for _, window := range []KeyUsageRankingWindow{report.Rankings.Account.Today, report.Rankings.Site.Last30d} {
		require.GreaterOrEqual(t, window.TotalKeys, window.SelfRank)
		require.Equal(t, 1, window.TotalKeys)
	}
}
