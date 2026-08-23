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
	mu      sync.Mutex
	byKey   map[string]*APIKey
	byID    map[int64]*APIKey
	lookups []string
}

func (f *fakeKeyUsageAPIKeys) GetByKey(_ context.Context, key string) (*APIKey, error) {
	f.mu.Lock()
	f.lookups = append(f.lookups, key)
	f.mu.Unlock()
	if apiKey, ok := f.byKey[key]; ok {
		return apiKey, nil
	}
	return nil, ErrAPIKeyNotFound
}

func (f *fakeKeyUsageAPIKeys) resetLookups() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = nil
}

func (f *fakeKeyUsageAPIKeys) keyLookups() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lookups...)
}

func (f *fakeKeyUsageAPIKeys) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if apiKey, ok := f.byID[id]; ok {
		return apiKey, nil
	}
	return nil, ErrAPIKeyNotFound
}

type fakeKeyUsageModelStats struct {
	mu     sync.Mutex
	byKey  map[int64][]usagestats.ModelStat
	ranges [][2]time.Time
	err    error
}

func (f *fakeKeyUsageModelStats) GetAPIKeyModelStats(_ context.Context, apiKeyID int64, start, end time.Time) ([]usagestats.ModelStat, error) {
	f.mu.Lock()
	f.ranges = append(f.ranges, [2]time.Time{start, end})
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.byKey[apiKeyID], nil
}

// queriedRanges 返回窗口汇总查询用过的时间区间快照。
func (f *fakeKeyUsageModelStats) queriedRanges() [][2]time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]time.Time(nil), f.ranges...)
}

type fakeKeyUsageRanking struct {
	mu        sync.Mutex
	rows      []usagestats.APIKeyUsageAggregate
	names     map[int64]string
	aggCalls  int
	siteCalls int
	// ranges 记录每次聚合查询实际用到的 [start, end)，用来断言只有被请求的那个窗口被算过。
	ranges [][2]time.Time
	err    error
}

func (f *fakeKeyUsageRanking) GetAPIKeyUsageAggregates(_ context.Context, start, end time.Time, userID int64, metric string) ([]usagestats.APIKeyUsageAggregate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aggCalls++
	f.ranges = append(f.ranges, [2]time.Time{start, end})
	if userID == 0 {
		f.siteCalls++
	}
	if f.err != nil {
		return nil, f.err
	}
	return sortedAggregatesForMetric(f.rows, metric), nil
}

// queriedRanges 返回聚合查询用过的时间区间快照。
func (f *fakeKeyUsageRanking) queriedRanges() [][2]time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]time.Time(nil), f.ranges...)
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

// testKeyUsageClientIP 是测试里默认的来源 IP（不受任何 Key 的 IP ACL 限制）。
const testKeyUsageClientIP = "203.0.113.10"

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

	_, missingErr := svc.ResolveRawKey(context.Background(), "sk-does-not-exist", testKeyUsageClientIP)
	_, disabledErr := svc.ResolveRawKey(context.Background(), "sk-disabled", testKeyUsageClientIP)
	_, emptyErr := svc.ResolveRawKey(context.Background(), "   ", testKeyUsageClientIP)

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

	got, err := svc.ResolveRawKey(context.Background(), "sk-exhausted", testKeyUsageClientIP)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.ID)

	got, err = svc.ResolveRawKey(context.Background(), "sk-expired", testKeyUsageClientIP)
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

	token, expiresAt, err := svc.IssueToken(ctx, "sk-live", testKeyUsageClientIP)
	require.NoError(t, err)
	require.True(t, expiresAt.After(timezone.Now()))

	resolved, err := svc.ResolveToken(ctx, token, testKeyUsageClientIP)
	require.NoError(t, err)
	require.Equal(t, int64(11), resolved.ID)

	// 令牌与网关凭证必须是两套互不通用的东西。
	// 注意：只断言 "ResolveRawKey(token) 报错" 是同义反复——fake 的 byKey 里本来就没有
	// 这个字符串，任何随机串都能让它通过。这里真正验证三件事：
	//   1. 令牌里不含原始 key（不能靠拿到令牌反推出凭证）；
	//   2. ResolveRawKey 确实拿令牌去查了一次 Key 表（没有被提前短路），而 Key 表不认它；
	//   3. 反方向也不通：原始 API Key 不能当令牌用（否则 ?token=sk-... 就成了新的泄漏面）。
	t.Run("令牌与网关凭证互不通用", func(t *testing.T) {
		require.NotContains(t, token, "sk-live")

		keys.resetLookups()
		_, err := svc.ResolveRawKey(ctx, token, testKeyUsageClientIP)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
		require.Equal(t, []string{token}, keys.keyLookups(),
			"必须真的把令牌当作 key 去查了一次，才能证明 Key 表不认它")

		// 反方向：原始 key 不是合法令牌（签名/结构都对不上）。
		_, err = svc.ResolveToken(ctx, apiKey.Key, testKeyUsageClientIP)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)

		// 令牌本身也不是一把可用的 key：即便把它塞进 Key 表，它也只是另一把 key，
		// 而当前这把 key 的令牌解析路径与 key 查询路径没有任何交叉。
		require.NotEqual(t, apiKey.Key, token)
	})

	t.Run("key 被禁用后立即失效", func(t *testing.T) {
		apiKey.Status = StatusAPIKeyDisabled
		defer func() { apiKey.Status = StatusAPIKeyActive }()
		_, err := svc.ResolveToken(ctx, token, testKeyUsageClientIP)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("key 被删除后立即失效", func(t *testing.T) {
		delete(keys.byID, 11)
		defer func() { keys.byID[11] = apiKey }()
		_, err := svc.ResolveToken(ctx, token, testKeyUsageClientIP)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("key 值轮换后旧令牌失效", func(t *testing.T) {
		original := apiKey.Key
		apiKey.Key = "sk-rotated"
		defer func() { apiKey.Key = original }()
		_, err := svc.ResolveToken(ctx, token, testKeyUsageClientIP)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})

	t.Run("被禁用的 key 换不到令牌", func(t *testing.T) {
		apiKey.Status = StatusAPIKeyDisabled
		defer func() { apiKey.Status = StatusAPIKeyActive }()
		_, _, err := svc.IssueToken(ctx, "sk-live", testKeyUsageClientIP)
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	})
}

// --- 窗口口径 -------------------------------------------------------------

// 窗口边界必须与按天曲线同源：起点是自然日 00:00，终点是明天 00:00。
func TestKeyUsageWindowRangesUseCalendarDayBoundaries(t *testing.T) {
	now := timezone.Now()
	ranges := KeyUsageWindowRanges(now, "")

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
	ranges := KeyUsageWindowRanges(now, "")

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

	report := svc.BuildReport(context.Background(), apiKey, "", KeyUsageWindowToday, "")
	require.Equal(t, KeyUsageWindowToday, report.Window)
	require.Equal(t, int64(12), report.WindowStat.Requests)
	require.Equal(t, int64(5300), report.WindowStat.Tokens)
	require.InDelta(t, 0.6, report.WindowStat.CostUSD, 1e-9)
	require.Len(t, report.WindowStat.Models, 2)
	require.Equal(t, "claude-opus-5", report.WindowStat.Models[0].Model)
	// 每个窗口的结构一致（这份 fake 对任何区间都返回同一批模型行）
	for _, window := range []string{KeyUsageWindowLast7d, KeyUsageWindowLast30d, KeyUsageWindowAll} {
		other := svc.BuildReport(context.Background(), apiKey, "", window, "")
		require.Equal(t, window, other.Window)
		require.Equal(t, report.WindowStat, other.WindowStat)
	}
}

func TestKeyUsageEmptyDataReturnsZeroValues(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	ranking := &fakeKeyUsageRanking{}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, &fakeKeyUsageModelStats{}, ranking, time.Minute)

	// 四个窗口（含全时段）在完全无数据时都必须给出同一套零值，而不是 nil / 空排名。
	for _, window := range []string{KeyUsageWindowToday, KeyUsageWindowLast7d, KeyUsageWindowLast30d, KeyUsageWindowAll} {
		report := svc.BuildReport(context.Background(), apiKey, "", window, "")
		require.Equal(t, window, report.Window)
		require.Equal(t, int64(0), report.WindowStat.Requests)
		require.NotNil(t, report.WindowStat.Models)
		require.Empty(t, report.WindowStat.Models)

		for _, scope := range []KeyUsageRankingWindow{report.Rankings.Account, report.Rankings.Site} {
			require.NotNil(t, scope.Top)
			require.Empty(t, scope.Top)
			// 自己没有用量时也要出现在总数里，避免 self_rank > total_keys
			require.Equal(t, 1, scope.TotalKeys)
			require.Equal(t, 1, scope.SelfRank)
			require.True(t, scope.Self.IsSelf)
			require.Equal(t, "self", scope.Self.KeyName)
		}
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

	report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	account := report.Rankings.Account

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
			report := svc.BuildReport(context.Background(), apiKey, tc.metric, KeyUsageWindowToday, "")
			require.Equal(t, tc.metric, report.Metric)
			require.Equal(t, tc.wantNames, namesOf(report.Rankings.Site.Top))
			require.Equal(t, tc.wantSelf, report.Rankings.Site.SelfRank)
		})
	}
}

func TestKeyUsageRankingFallsBackToCostForUnknownMetric(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), rankingFixture(), time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, "DROP TABLE usage_logs", KeyUsageWindowToday, "")
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

	site := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "").Rankings.Site
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

	svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	require.Equal(t, 1, ranking.siteCalls, "首次请求：只有被请求的那个窗口查一次全站榜")

	// 换一把 key（不同账户）也应命中同一份全站缓存
	svc.BuildReport(ctx, apiKeyB, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	require.Equal(t, 1, ranking.siteCalls, "全站榜与具体 Key 无关，必须复用缓存")

	// 换 metric 必须重新聚合，不能串味
	svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricTokens, KeyUsageWindowToday, "")
	require.Equal(t, 2, ranking.siteCalls)

	// 换窗口同样必须重新聚合：缓存 key 里编了窗口边界
	svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricCost, KeyUsageWindowAll, "")
	require.Equal(t, 3, ranking.siteCalls)

	tokensTop := svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricTokens, KeyUsageWindowToday, "").Rankings.Site
	costTop := svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "").Rankings.Site
	require.Equal(t, 3, ranking.siteCalls, "两个 metric 都已缓存")
	require.Equal(t, "third", tokensTop.Top[0].KeyName)
	require.Equal(t, "top", costTop.Top[0].KeyName)
}

func TestKeyUsageSiteRankingCacheExpires(t *testing.T) {
	ranking := rankingFixture()
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, 20*time.Millisecond)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	ctx := context.Background()

	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	require.Equal(t, 1, ranking.siteCalls)

	time.Sleep(40 * time.Millisecond)
	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	require.Equal(t, 2, ranking.siteCalls, "TTL 过期后必须重新聚合")
}

// 账户内榜每次都实时查（数据量小），不吃全站缓存。
func TestKeyUsageAccountRankingIsNotCached(t *testing.T) {
	ranking := rankingFixture()
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	ctx := context.Background()

	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	svc.BuildReport(ctx, apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")

	ranking.mu.Lock()
	defer ranking.mu.Unlock()
	require.Equal(t, 1, ranking.siteCalls)
	require.Equal(t, 3, ranking.aggCalls, "账户榜 2 次（不缓存）+ 全站榜 1 次（缓存命中）")
}

// 排名查询失败时页面其余部分照常渲染，排名板块必须明确标记为"不可用"。
//
// 这里绝不能返回 self_rank=1 / total_keys=1：那会让每一个访客在 DB 抖动时
// 都被渲染成"全站第 1 / 共 1 个 Key"，且与真实数据在视觉上无法区分。
func TestKeyUsageRankingDegradesOnRepositoryError(t *testing.T) {
	ranking := &fakeKeyUsageRanking{err: context.DeadlineExceeded}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")

	for _, name := range []string{KeyUsageWindowToday, KeyUsageWindowLast7d, KeyUsageWindowLast30d, KeyUsageWindowAll} {
		report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, name, "")
		require.Equal(t, int64(5), report.WindowStat.Requests, "排名失败不影响窗口汇总")

		for _, window := range []KeyUsageRankingWindow{report.Rankings.Account, report.Rankings.Site} {
			require.NotNil(t, window.Top)
			require.Empty(t, window.Top)
			require.Equal(t, 0, window.SelfRank, "0 = 排名不可用")
			require.Equal(t, 0, window.TotalKeys, "不能凭空造出一个共 N 个 Key 的总数")
			require.Equal(t, 0, window.Self.Rank)
			require.True(t, window.Self.IsSelf)
			// 自己的用量仍然照常下发（这部分数据是好的）
			require.Equal(t, int64(5), window.Self.Requests)
		}
	}
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

	_, err := svc.ResolveRawKey(ctx, "sk-banned-owner", testKeyUsageClientIP)
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)

	// 令牌是在账号正常时签发的，封禁后必须立刻失效
	apiKey.User.Status = StatusActive
	token, _, err := svc.IssueToken(ctx, "sk-banned-owner", testKeyUsageClientIP)
	require.NoError(t, err)
	apiKey.User.Status = "disabled"
	_, err = svc.ResolveToken(ctx, token, testKeyUsageClientIP)
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
}

// total_keys 永远不小于 self_rank（前端会用它渲染"第 N / 共 M"）。
//
// 这个不变式现在由构造过程本身保证（名次 = 严格大于自己的行数 + 1 ≤ 行数，
// 自己不在聚合结果里时行数会 +1），不再需要一条 "if totalKeys < selfRank" 的事后钳制
// ——那行钳制会把 self_rank 算错这类根因掩盖成一个看起来合理的数字。
func TestKeyUsageTotalKeysNeverBelowSelfRank(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")

	cases := map[string]*fakeKeyUsageModelStats{
		"自己在榜上":    selfWithUsage(5, 500, 5),
		"自己零用量不在榜": {},
	}
	for name, stats := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, stats, rankingFixture(), time.Minute)
			for _, name := range []string{KeyUsageWindowToday, KeyUsageWindowLast7d, KeyUsageWindowLast30d, KeyUsageWindowAll} {
				report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, name, "")
				for _, window := range []KeyUsageRankingWindow{report.Rankings.Account, report.Rankings.Site} {
					require.Positive(t, window.SelfRank)
					require.GreaterOrEqual(t, window.TotalKeys, window.SelfRank)
				}
			}
		})
	}
}

// --- IP 白名单/黑名单（P0-2） -----------------------------------------------

func ipRestrictedTestKey(id int64, key string, whitelist, blacklist []string) *APIKey {
	apiKey := activeTestAPIKey(id, 1, key, "restricted")
	apiKey.IPWhitelist = whitelist
	apiKey.IPBlacklist = blacklist
	return apiKey
}

// 网关中间件对 /v1/usage 强制执行 Key 的 IP 白名单；免登录用量页必须同口径，
// 否则一把配了白名单的 Key 泄露后就能从任意 IP 读到余额/额度/套餐/逐日曲线。
func TestKeyUsageResolveRawKeyEnforcesIPWhitelist(t *testing.T) {
	apiKey := ipRestrictedTestKey(31, "sk-wl", []string{"203.0.113.0/24"}, nil)
	keys := &fakeKeyUsageAPIKeys{
		byKey: map[string]*APIKey{"sk-wl": apiKey},
		byID:  map[int64]*APIKey{31: apiKey},
	}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)
	ctx := context.Background()

	allowed, err := svc.ResolveRawKey(ctx, "sk-wl", "203.0.113.44")
	require.NoError(t, err)
	require.Equal(t, int64(31), allowed.ID)

	_, err = svc.ResolveRawKey(ctx, "sk-wl", "198.51.100.7")
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized, "白名单外的 IP 必须被拒")

	// 拒绝文案与"key 不存在"完全一致，不泄露这把 key 是否存在。
	_, missingErr := svc.ResolveRawKey(ctx, "sk-does-not-exist", "198.51.100.7")
	_, deniedErr := svc.ResolveRawKey(ctx, "sk-wl", "198.51.100.7")
	require.Equal(t, missingErr.Error(), deniedErr.Error())
}

func TestKeyUsageResolveRawKeyEnforcesIPBlacklist(t *testing.T) {
	apiKey := ipRestrictedTestKey(32, "sk-bl", nil, []string{"198.51.100.0/24"})
	keys := &fakeKeyUsageAPIKeys{byKey: map[string]*APIKey{"sk-bl": apiKey}, byID: map[int64]*APIKey{32: apiKey}}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)
	ctx := context.Background()

	_, err := svc.ResolveRawKey(ctx, "sk-bl", "198.51.100.7")
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)

	got, err := svc.ResolveRawKey(ctx, "sk-bl", "203.0.113.5")
	require.NoError(t, err)
	require.Equal(t, int64(32), got.ID)
}

// 令牌路径同样要查 IP ACL：否则签发出去的令牌就是一张不受 IP 限制、长期有效的旁路凭证。
func TestKeyUsageResolveTokenEnforcesIPRestriction(t *testing.T) {
	apiKey := ipRestrictedTestKey(33, "sk-tok", []string{"203.0.113.0/24"}, nil)
	keys := &fakeKeyUsageAPIKeys{byKey: map[string]*APIKey{"sk-tok": apiKey}, byID: map[int64]*APIKey{33: apiKey}}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)
	ctx := context.Background()

	token, _, err := svc.IssueToken(ctx, "sk-tok", "203.0.113.44")
	require.NoError(t, err)

	resolved, err := svc.ResolveToken(ctx, token, "203.0.113.90")
	require.NoError(t, err, "白名单内的 IP 继续可用")
	require.Equal(t, int64(33), resolved.ID)

	_, err = svc.ResolveToken(ctx, token, "198.51.100.7")
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized, "令牌不能绕过 IP 白名单")

	// 白名单外的 IP 连令牌都换不到。
	_, _, err = svc.IssueToken(ctx, "sk-tok", "198.51.100.7")
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
}

// 解析不出客户端 IP 时，配了 IP ACL 的 Key 必须 fail-close。
func TestKeyUsageIPRestrictionFailsClosedOnUnknownIP(t *testing.T) {
	apiKey := ipRestrictedTestKey(34, "sk-unknown-ip", []string{"203.0.113.0/24"}, nil)
	keys := &fakeKeyUsageAPIKeys{byKey: map[string]*APIKey{"sk-unknown-ip": apiKey}, byID: map[int64]*APIKey{34: apiKey}}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)

	_, err := svc.ResolveRawKey(context.Background(), "sk-unknown-ip", "")
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
}

// 没有配置任何 IP 规则的 Key 不受影响（绝大多数 Key 都是这种）。
func TestKeyUsageWithoutIPRulesIgnoresClientIP(t *testing.T) {
	apiKey := activeTestAPIKey(35, 1, "sk-open", "open")
	keys := &fakeKeyUsageAPIKeys{byKey: map[string]*APIKey{"sk-open": apiKey}, byID: map[int64]*APIKey{35: apiKey}}
	svc := newTestKeyUsageService(t, keys, &fakeKeyUsageModelStats{}, nil, time.Minute)

	got, err := svc.ResolveRawKey(context.Background(), "sk-open", "")
	require.NoError(t, err)
	require.Equal(t, int64(35), got.ID)
}

// --- self_rank 与榜单必须同源（P1-1） ---------------------------------------

// buildWindowStat 在 Go 里逐模型累加 ActualCost，而榜单值来自 SQL 的 SUM(actual_cost)：
// 两者求和顺序不同，末位比特就不同。用 Go 侧的和去二分榜单，会把自己算进
// "严格大于自己"的那一堆里，名次凭空 +1 —— 能复现出"金牌是自己、同时显示第 2 名 / 共 2 个"。
// 名次必须直接取聚合结果里自己那一行的值。
func TestKeyUsageSelfRankUsesAggregateValueNotRecomputedSum(t *testing.T) {
	// 注意用变量累加而不是字面量表达式：常量表达式会被编译期精确求值，
	// 那样就复现不出运行期浮点求和的顺序差异了。
	costs := []float64{0.1, 0.2, 0.3}
	// SQL 侧（榜单）的求和顺序：0.1 + 0.2 + 0.3
	sqlSum := 0.0
	for _, cost := range costs {
		sqlSum += cost
	}
	// Go 侧（buildWindowStat）按模型行顺序累加：0.3 + 0.2 + 0.1
	goSum := 0.0
	for i := len(costs) - 1; i >= 0; i-- {
		goSum += costs[i]
	}
	require.NotEqual(t, sqlSum, goSum, "前提：两种求和顺序的末位比特不同")
	require.Less(t, goSum, sqlSum)

	ranking := &fakeKeyUsageRanking{
		rows: []usagestats.APIKeyUsageAggregate{
			{APIKeyID: 1, Requests: 3, Tokens: 300, Cost: sqlSum},
			{APIKeyID: 2, Requests: 1, Tokens: 100, Cost: 0.5},
		},
		names: map[int64]string{1: "self", 2: "other"},
	}
	stats := &fakeKeyUsageModelStats{byKey: map[int64][]usagestats.ModelStat{
		1: {
			{Model: "m-a", Requests: 1, TotalTokens: 100, ActualCost: 0.3},
			{Model: "m-b", Requests: 1, TotalTokens: 100, ActualCost: 0.2},
			{Model: "m-c", Requests: 1, TotalTokens: 100, ActualCost: 0.1},
		},
	}}
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, stats, ranking, time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "")
	for _, window := range []KeyUsageRankingWindow{report.Rankings.Account, report.Rankings.Site} {
		require.Equal(t, 2, window.TotalKeys)
		require.Equal(t, 1, window.SelfRank, "自己就是榜首，名次必须是 1")
		require.Equal(t, 1, window.Self.Rank)
		require.True(t, window.Top[0].IsSelf, "前提：榜首那一行确实是自己")
		// 金牌是自己 ⇒ 名次必须也是 1，不允许出现自相矛盾的组合
		require.Equal(t, window.Top[0].Rank, window.SelfRank)
	}
}

// 自己不在聚合结果里（窗口内零用量）时回落到实时值，并把自己补进总数。
func TestKeyUsageSelfRankFallsBackWhenAbsentFromAggregates(t *testing.T) {
	ranking := &fakeKeyUsageRanking{
		rows:  []usagestats.APIKeyUsageAggregate{{APIKeyID: 2, Requests: 10, Tokens: 100, Cost: 9}},
		names: map[int64]string{2: "other"},
	}
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, &fakeKeyUsageModelStats{}, ranking, time.Minute)

	window := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowToday, "").Rankings.Site
	require.Equal(t, 2, window.TotalKeys)
	require.Equal(t, 2, window.SelfRank)
}

// --- 全站榜缓存回收（P1-2） -------------------------------------------------

// 缓存 key 里编了窗口边界的时间戳，跨天后昨天的 key 永远不会被再次读取。
// 只靠 get 的惰性删除，map 会无限增长；set 必须顺手回收过期项。
func TestKeyUsageSiteCacheReclaimsExpiredEntriesOnSet(t *testing.T) {
	cache := newKeyUsageSiteCache(20 * time.Millisecond)
	for day := 0; day < 9; day++ {
		cache.set("cost|today|day-"+strconv.Itoa(day), &keyUsageSiteSnapshot{Values: []float64{1}})
	}
	require.Equal(t, 9, cache.Len())

	time.Sleep(40 * time.Millisecond)
	cache.set("cost|today|day-next", &keyUsageSiteSnapshot{Values: []float64{1}})
	require.Equal(t, 1, cache.Len(), "过期的旧窗口条目必须在写入时被回收")
}

// 兜底上限：即使 TTL 被配得很长（条目都没过期），缓存也不允许无界增长。
func TestKeyUsageSiteCacheIsBounded(t *testing.T) {
	cache := newKeyUsageSiteCache(time.Hour)
	for i := 0; i < keyUsageSiteCacheMaxEntries*3; i++ {
		cache.set("k-"+strconv.Itoa(i), &keyUsageSiteSnapshot{Values: []float64{1}})
	}
	require.LessOrEqual(t, cache.Len(), keyUsageSiteCacheMaxEntries)
}

// --- 窗口时区口径（P2-1） ---------------------------------------------------

// 前端每次都会带上浏览器时区，窗口边界必须跟着它切；
// 否则 UTC+8 的访客在 UTC 服务器上看到的"今日汇总"与按天曲线最后一行差 8 小时。
func TestKeyUsageWindowRangesFollowUserTimezone(t *testing.T) {
	now := time.Date(2026, time.March, 2, 20, 30, 0, 0, time.UTC)

	shanghai := KeyUsageWindowRanges(now, "Asia/Shanghai")
	losAngeles := KeyUsageWindowRanges(now, "America/Los_Angeles")
	require.NotEqual(t, shanghai[KeyUsageWindowToday], losAngeles[KeyUsageWindowToday],
		"不同浏览器时区必须切出不同的自然日边界")

	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	// UTC 2026-03-02 20:30 == 上海时间 2026-03-03 04:30，自然日是 3 月 3 日
	require.True(t, shanghai[KeyUsageWindowToday][0].Equal(time.Date(2026, time.March, 3, 0, 0, 0, 0, loc)))
	require.True(t, shanghai[KeyUsageWindowToday][1].Equal(time.Date(2026, time.March, 4, 0, 0, 0, 0, loc)))
	require.True(t, shanghai[KeyUsageWindowLast7d][0].Equal(time.Date(2026, time.February, 25, 0, 0, 0, 0, loc)))
	require.True(t, shanghai[KeyUsageWindowLast30d][0].Equal(time.Date(2026, time.February, 2, 0, 0, 0, 0, loc)))
}

// --- 全时段（all）窗口 -----------------------------------------------------

// window 参数来自免登录页的 query string，必须能吃下任意输入并落到已知窗口上。
// 非法值回落到 today 而不是 400：一个拼错的参数不该把整页打成错误页。
func TestNormalizeKeyUsageWindow(t *testing.T) {
	cases := map[string]string{
		KeyUsageWindowToday:   KeyUsageWindowToday,
		KeyUsageWindowLast7d:  KeyUsageWindowLast7d,
		KeyUsageWindowLast30d: KeyUsageWindowLast30d,
		KeyUsageWindowAll:     KeyUsageWindowAll,
		"  ALL  ":             KeyUsageWindowAll,
		"Last_30D":            KeyUsageWindowLast30d,
		"":                    KeyUsageWindowToday,
		"   ":                 KeyUsageWindowToday,
		"bogus":               KeyUsageWindowToday,
		"1 OR 1=1":            KeyUsageWindowToday,
		"last_90d":            KeyUsageWindowToday,
	}
	for input, want := range cases {
		require.Equal(t, want, NormalizeKeyUsageWindow(input), "input=%q", input)
	}
}

// all 的边界：下界是固定纪元（不随请求漂移），上界与其它窗口共用同一个量化过的值。
func TestKeyUsageAllWindowBounds(t *testing.T) {
	now := time.Date(2026, time.March, 2, 3, 14, 15, 0, timezone.Location())
	ranges := KeyUsageWindowRanges(now, "")

	all, ok := ranges[KeyUsageWindowAll]
	require.True(t, ok, "all 必须是一个已知窗口")
	require.True(t, all[0].Equal(keyUsageAllWindowStart), "下界必须是固定纪元")
	require.True(t, all[0].Before(ranges[KeyUsageWindowLast30d][0]), "全时段起点必须早于 30 天窗口")
	require.True(t, all[1].Equal(ranges[KeyUsageWindowToday][1]), "上界与其它窗口共用同一个自然日边界")
}

// 缓存 key 编了窗口边界，all 的边界必须在同一自然日内保持稳定：
// 只要这里退化成 now，all 就永远命不中缓存，每次页面加载都跑一遍全表 GROUP BY。
func TestKeyUsageAllWindowCacheKeyIsQuantizedPerDay(t *testing.T) {
	loc := timezone.Location()
	keyAt := func(now time.Time) string {
		return keyUsageSiteCacheKey(usagestats.KeyRankingMetricCost, keyUsageWindowFor(KeyUsageWindowAll, now, ""))
	}

	justAfterMidnight := time.Date(2026, time.March, 2, 0, 0, 1, 0, loc)
	noon := time.Date(2026, time.March, 2, 12, 0, 0, 0, loc)
	justBeforeMidnight := time.Date(2026, time.March, 2, 23, 59, 59, 0, loc)
	nextDay := time.Date(2026, time.March, 3, 0, 0, 1, 0, loc)

	require.Equal(t, keyAt(justAfterMidnight), keyAt(noon))
	require.Equal(t, keyAt(noon), keyAt(justBeforeMidnight))
	require.NotEqual(t, keyAt(noon), keyAt(nextDay), "跨天后必须换 key，否则会一直读到昨天的榜")
}

// 同一时段内反复请求 all，全站聚合只允许回源一次。
func TestKeyUsageAllWindowSiteRankingLoadsOnce(t *testing.T) {
	ranking := rankingFixture()
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, selfWithUsage(5, 500, 5), ranking, time.Minute)
	apiKeyA := activeTestAPIKey(1, 1, "sk-1", "self")
	apiKeyB := activeTestAPIKey(9, 2, "sk-9", "other")
	ctx := context.Background()

	for range 5 {
		svc.BuildReport(ctx, apiKeyA, usagestats.KeyRankingMetricCost, KeyUsageWindowAll, "")
	}
	// 换一把属于其它账户的 key 同样必须命中同一份缓存（全站榜与具体 Key 无关）。
	svc.BuildReport(ctx, apiKeyB, usagestats.KeyRankingMetricCost, KeyUsageWindowAll, "")

	ranking.mu.Lock()
	require.Equal(t, 1, ranking.siteCalls, "同一时段内多次请求 all 只允许跑一次全表聚合")
	ranking.mu.Unlock()

	// 上面那段在同一秒内跑完，即使窗口边界退化成 now 也会碰巧命中（Unix() 只到秒）。
	// 真正要锁住的是"一整天之内到达的请求共享同一份快照"，所以这里直接按不同时刻
	// 构造窗口，绕开不可注入的 timezone.Now()。
	t.Run("同一自然日内不同时刻的请求共享同一份快照", func(t *testing.T) {
		loc := timezone.Location()
		arrivals := []time.Time{
			time.Date(2026, time.March, 2, 0, 0, 1, 0, loc),
			time.Date(2026, time.March, 2, 7, 13, 59, 0, loc),
			time.Date(2026, time.March, 2, 12, 0, 0, 0, loc),
			time.Date(2026, time.March, 2, 23, 59, 59, 0, loc),
		}

		cache := newKeyUsageSiteCache(time.Minute)
		loads := 0
		load := func() (*keyUsageSiteSnapshot, error) {
			loads++
			return &keyUsageSiteSnapshot{Values: []float64{1}}, nil
		}

		for _, at := range arrivals {
			window := keyUsageWindowFor(KeyUsageWindowAll, at, "")
			snapshot, err := cache.GetOrLoad(keyUsageSiteCacheKey(usagestats.KeyRankingMetricCost, window), cache.ttlFor(window), load)
			require.NoError(t, err)
			require.NotNil(t, snapshot)
		}

		require.Equal(t, 1, loads, "同一自然日内的多次 all 请求只允许回源一次")
		require.Equal(t, 1, cache.Len(), "而且只能占用一个缓存条目")
	})
}

// all 是最贵的一种聚合，同时相对变化最慢：它的缓存 TTL 必须比其它窗口长。
func TestKeyUsageAllWindowUsesLongerCacheTTL(t *testing.T) {
	cache := newKeyUsageSiteCache(time.Minute)
	now := timezone.Now()

	shortTTL := cache.ttlFor(keyUsageWindowFor(KeyUsageWindowToday, now, ""))
	longTTL := cache.ttlFor(keyUsageWindowFor(KeyUsageWindowAll, now, ""))

	require.Equal(t, time.Minute, shortTTL)
	require.Equal(t, time.Minute*keyUsageAllWindowCacheTTLFactor, longTTL)
	require.Greater(t, longTTL, shortTTL)
}

// all 窗口的汇总与排名口径与其它窗口完全一致（同一套聚合代码，只是区间更宽）。
func TestKeyUsageAllWindowAggregatesAndRanks(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	stats := &fakeKeyUsageModelStats{byKey: map[int64][]usagestats.ModelStat{
		1: {
			{Model: "claude-opus-5", Requests: 40, TotalTokens: 1_200_000, ActualCost: 12.5},
			{Model: "claude-haiku-5", Requests: 60, TotalTokens: 800_000, ActualCost: 2.5},
		},
	}}
	svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, stats, rankingFixture(), time.Minute)

	report := svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, KeyUsageWindowAll, "")

	require.Equal(t, KeyUsageWindowAll, report.Window)
	require.Equal(t, int64(100), report.WindowStat.Requests)
	require.Equal(t, int64(2_000_000), report.WindowStat.Tokens)
	require.InDelta(t, 15.0, report.WindowStat.CostUSD, 1e-9)
	require.Len(t, report.WindowStat.Models, 2)

	for _, window := range []KeyUsageRankingWindow{report.Rankings.Account, report.Rankings.Site} {
		require.Equal(t, 4, window.TotalKeys)
		require.Equal(t, []string{"top", "third", "fourth", "self"}, namesOf(window.Top))
		require.Equal(t, 4, window.SelfRank)
		require.True(t, window.Self.IsSelf)
	}

}

// 只计算被请求的那一个窗口：其它三个窗口的区间一次都不能出现在仓储调用里。
//
// 这是"加了 all 之后不要每次都算四个窗口"的核心性质——把它写死在测试里，
// 否则以后有人为了省事把四个窗口一起算回来，代价（每次请求跑一遍全表聚合）不会有任何信号。
func TestKeyUsageBuildReportQueriesOnlyRequestedWindow(t *testing.T) {
	apiKey := activeTestAPIKey(1, 1, "sk-1", "self")
	all := []string{KeyUsageWindowToday, KeyUsageWindowLast7d, KeyUsageWindowLast30d, KeyUsageWindowAll}

	for _, name := range all {
		t.Run(name, func(t *testing.T) {
			stats := selfWithUsage(5, 500, 5)
			ranking := rankingFixture()
			svc := newTestKeyUsageService(t, &fakeKeyUsageAPIKeys{}, stats, ranking, time.Minute)

			svc.BuildReport(context.Background(), apiKey, usagestats.KeyRankingMetricCost, name, "")

			ranges := KeyUsageWindowRanges(timezone.Now(), "")
			want := ranges[name]

			queried := append(stats.queriedRanges(), ranking.queriedRanges()...)
			require.NotEmpty(t, queried, "前提：确实发生了查询")
			for _, got := range queried {
				require.True(t, got[0].Equal(want[0]) && got[1].Equal(want[1]),
					"查询区间 %v 不属于被请求的窗口 %s (%v)", got, name, want)
			}

			// 反向断言：另外三个窗口的起点一次都没被查过。
			for _, other := range all {
				if other == name {
					continue
				}
				otherStart := ranges[other][0]
				if otherStart.Equal(want[0]) {
					continue // today 与 last_7d 在跨度上不同，起点不会相等；防御性跳过
				}
				for _, got := range queried {
					require.False(t, got[0].Equal(otherStart),
						"窗口 %s 的区间被计算了，但本次只请求了 %s", other, name)
				}
			}
		})
	}
}
