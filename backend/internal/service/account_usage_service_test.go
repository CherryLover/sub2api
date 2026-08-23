package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type accountUsageCodexProbeRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
}

func (r *accountUsageCodexProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

func (r *accountUsageCodexProbeRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func TestShouldRefreshOpenAICodexSnapshot(t *testing.T) {
	t.Parallel()

	rateLimitedUntil := time.Now().Add(5 * time.Minute)
	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{RateLimitResetAt: &rateLimitedUntil}, usage, now) {
		t.Fatal("expected rate-limited account to force codex snapshot refresh")
	}

	if shouldRefreshOpenAICodexSnapshot(&Account{}, usage, now) {
		t.Fatal("expected complete non-rate-limited usage to skip codex snapshot refresh")
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{}, &UsageInfo{FiveHour: nil, SevenDay: &UsageProgress{}}, now) {
		t.Fatal("expected missing 5h snapshot to require refresh")
	}

	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	if !shouldRefreshOpenAICodexSnapshot(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       staleAt,
		},
	}, usage, now) {
		t.Fatal("expected stale ws snapshot to trigger refresh")
	}
}

// TestShouldRefreshOpenAICodexSnapshot_SparkShadowIgnoresWSv2 外审第9轮 P1:spark 影子用量走
// QueryUsage(/wham/usage,与 WSv2 无关),staleness 不得被 WSv2 门控,否则首刷后窗口永久冻结。
func TestShouldRefreshOpenAICodexSnapshot_SparkShadowIgnoresWSv2(t *testing.T) {
	t.Parallel()

	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}
	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	freshAt := now.Add(-time.Minute).Format(time.RFC3339)
	parentID := int64(7001)

	// 影子无 WSv2,但首刷后窗口已存在;过期 codex_usage_updated_at 必须触发再刷新。
	shadowStale := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Extra:           map[string]any{"codex_usage_updated_at": staleAt},
	}
	if !shouldRefreshOpenAICodexSnapshot(shadowStale, usage, now) {
		t.Fatal("expected stale spark shadow (no WSv2) to trigger refresh")
	}

	// 影子时间戳仍新鲜→不刷(TTL 生效)。
	shadowFresh := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Extra:           map[string]any{"codex_usage_updated_at": freshAt},
	}
	if shouldRefreshOpenAICodexSnapshot(shadowFresh, usage, now) {
		t.Fatal("expected fresh spark shadow to skip refresh (TTL not elapsed)")
	}

	// 反向对照:普通账号无 WSv2 + 过期时间戳→仍不刷(WSv2 门控普通账号的 probe 刷新)。
	normalNoWS := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_usage_updated_at": staleAt},
	}
	if shouldRefreshOpenAICodexSnapshot(normalNoWS, usage, now) {
		t.Fatal("expected non-WSv2 normal account to skip codex probe refresh")
	}
}

func TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, err := extractOpenAICodexProbeUpdates(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headers})
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeUpdates() error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
	}
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

func TestAccountUsageService_PersistOpenAICodexProbeSnapshotOnlyUpdatesExtra(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	svc.persistOpenAICodexProbeSnapshot(321, map[string]any{
		"codex_7d_used_percent": 100.0,
		"codex_7d_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
	})

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_7d_used_percent"]; got != 100.0 {
			t.Fatalf("codex_7d_used_percent = %v, want 100", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 探测快照写入 extra 超时")
	}

	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将探测快照写入运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAccountUsageService_GetOpenAIUsage_DoesNotPromoteCodexExtraToRateLimit(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(6 * 24 * time.Hour).UTC().Truncate(time.Second)
	repo := &accountUsageCodexProbeRepo{
		rateLimitCh: make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 1.0,
			"codex_5h_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	}

	usage, err := svc.getOpenAIUsage(context.Background(), account, false)
	if err != nil {
		t.Fatalf("getOpenAIUsage() error = %v", err)
	}
	if usage.SevenDay == nil || usage.SevenDay.Utilization != 100.0 {
		t.Fatalf("预期 7 天用量仍然可见，实际为 %#v", usage.SevenDay)
	}
	if account.RateLimitResetAt != nil {
		t.Fatalf("不应让已耗尽的 codex extra 改写运行时限流状态: %v", account.RateLimitResetAt)
	}
	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将已耗尽的 codex extra 持久化为运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestBuildCodexUsageProgressFromExtra_ZerosExpiredWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	t.Run("expired 5h window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     "2026-03-16T10:00:00Z", // 2h ago
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired window, got %v", progress.Utilization)
		}
		if progress.RemainingSeconds != 0 {
			t.Fatalf("expected RemainingSeconds=0, got %v", progress.RemainingSeconds)
		}
	})

	t.Run("active 5h window keeps utilization", func(t *testing.T) {
		resetAt := now.Add(2 * time.Hour).Format(time.RFC3339)
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     resetAt,
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 42.0 {
			t.Fatalf("expected Utilization=42, got %v", progress.Utilization)
		}
	})

	t.Run("expired 7d window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     "2026-03-15T00:00:00Z", // yesterday
		}
		progress := buildCodexUsageProgressFromExtra(extra, "7d", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired 7d window, got %v", progress.Utilization)
		}
	})
}

// openAISelfHealRepo 忠实模拟 account_repo.go 里两个清除原语【副作用面的差异】。
// 假 repo 只记录 ID 是不够的：P1-2（顺带抹掉 529 过载冷却）和 P0-1（丢失更新）都只在
// 副作用被如实模拟时才暴露得出来。
//
//   - ClearRateLimit：裸 UPDATE ... WHERE id，且额外 ClearOverloadUntil()；
//   - ClearOpenAIRateLimitIfObserved：WHERE 还匹配 (rate_limited_at, rate_limit_reset_at)
//     这一代，且只清 rate_limit_*，绝不动 overload_until。
//
// state 代表"数据库里的那一行"，与调用方手里的内存快照分离——这样"probe 飞行期间限流被改写"
// 就能被真实地表达出来。
type openAISelfHealRepo struct {
	stubOpenAIAccountRepo

	// state 是被清除操作作用的那一行；nil 表示测试不关心行状态。
	state *Account

	unconditionalClears []int64
	observedClears      []openAISelfHealObservedClear
	extraUpdates        []map[string]any
	observedErr         error
}

type openAISelfHealObservedClear struct {
	id        int64
	limitedAt time.Time
	resetAt   time.Time
	cleared   bool
}

func (r *openAISelfHealRepo) ClearRateLimit(_ context.Context, id int64) error {
	r.unconditionalClears = append(r.unconditionalClears, id)
	if r.state != nil {
		r.state.RateLimitedAt = nil
		r.state.RateLimitResetAt = nil
		// 真实实现确实会连 529 过载冷却一起抹掉。
		r.state.OverloadUntil = nil
	}
	return nil
}

func (r *openAISelfHealRepo) ClearOpenAIRateLimitIfObserved(_ context.Context, id int64, observedLimitedAt, observedResetAt time.Time) (bool, error) {
	if r.observedErr != nil {
		r.observedClears = append(r.observedClears, openAISelfHealObservedClear{id: id, limitedAt: observedLimitedAt, resetAt: observedResetAt})
		return false, r.observedErr
	}
	cleared := false
	if r.state != nil &&
		r.state.ID == id &&
		r.state.Platform == PlatformOpenAI &&
		r.state.Type == AccountTypeOAuth &&
		r.state.RateLimitedAt != nil && r.state.RateLimitedAt.Equal(observedLimitedAt) &&
		r.state.RateLimitResetAt != nil && r.state.RateLimitResetAt.Equal(observedResetAt) {
		r.state.RateLimitedAt = nil
		r.state.RateLimitResetAt = nil
		// 刻意不动 OverloadUntil —— 这正是条件清除原语与 ClearRateLimit 的关键差异。
		cleared = true
	}
	r.observedClears = append(r.observedClears, openAISelfHealObservedClear{
		id: id, limitedAt: observedLimitedAt, resetAt: observedResetAt, cleared: cleared,
	})
	return cleared, nil
}

func (r *openAISelfHealRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
	}
	r.extraUpdates = append(r.extraUpdates, copied)
	return nil
}

func (r *openAISelfHealRepo) clearedCount() int {
	n := 0
	for _, c := range r.observedClears {
		if c.cleared {
			n++
		}
	}
	return n
}

// healthyCodexUpdates 是"本轮 probe 同时给出了 5h 与 7d 两个窗口"的最小 updates。
func healthyCodexUpdates() map[string]any {
	return map[string]any{
		"codex_5h_used_percent": 12.0,
		"codex_7d_used_percent": 40.0,
	}
}

// TestAccountUsageService_ClearOpenAIRateLimitIfCodexSnapshotHealthy 覆盖 OpenAI 的 429 自愈：
// 新鲜 codex 快照显示两个窗口都没耗尽时，被误标限流的账号应被解封；
// 窗口确实耗尽、对象是 spark 影子 / 非 OpenAI 账号、限流代际已变化、限流是 body 额度派生时，
// 一律不得清除。并且任何情况下都不得走无条件的 ClearRateLimit（它会顺带抹掉 529 过载冷却）。
func TestAccountUsageService_ClearOpenAIRateLimitIfCodexSnapshotHealthy(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(96 * time.Hour).UTC().Truncate(time.Second)
	limitedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	parentID := int64(4200)

	// newLimitedAccount 返回 (内存快照, 假 repo)，两者的限流代际初始一致。
	newLimitedAccount := func(id int64, platform string) (*Account, *openAISelfHealRepo) {
		mk := func() *Account {
			limited := limitedAt
			reset := future
			return &Account{
				ID:               id,
				Platform:         platform,
				Type:             AccountTypeOAuth,
				RateLimitedAt:    &limited,
				RateLimitResetAt: &reset,
			}
		}
		return mk(), &openAISelfHealRepo{state: mk()}
	}

	healthy := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 12},
		SevenDay: &UsageProgress{Utilization: 40},
	}

	t.Run("clears when neither window is exhausted", func(t *testing.T) {
		account, repo := newLimitedAccount(1, PlatformOpenAI)
		svc := &AccountUsageService{accountRepo: repo}

		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if repo.clearedCount() != 1 {
			t.Fatalf("observedClears = %+v, want exactly one successful conditional clear", repo.observedClears)
		}
		if len(repo.unconditionalClears) != 0 {
			t.Fatalf("unconditionalClears = %v, self-heal must never use the unconditional primitive", repo.unconditionalClears)
		}
		if account.RateLimitResetAt != nil || account.RateLimitedAt != nil {
			t.Fatal("in-memory rate limit fields should be cleared as well")
		}
		if account.IsRateLimited() {
			t.Fatal("account should no longer be rate limited")
		}
		if repo.state.RateLimitResetAt != nil {
			t.Fatal("row should no longer be rate limited")
		}
	})

	// P1-2：529 过载冷却与 codex 的 5h/7d used_percent 毫无关系，自愈不得把它一起抹掉，
	// 否则一个仍在过载的账号会被提前放回调度池。
	t.Run("keeps the 529 overload cooldown", func(t *testing.T) {
		account, repo := newLimitedAccount(10, PlatformOpenAI)
		overloadUntil := time.Now().Add(9 * time.Minute)
		account.OverloadUntil = &overloadUntil
		repo.state.OverloadUntil = &overloadUntil
		account.Status = StatusActive
		account.Schedulable = true
		repo.state.Status = StatusActive
		repo.state.Schedulable = true

		svc := &AccountUsageService{accountRepo: repo}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if repo.clearedCount() != 1 {
			t.Fatalf("observedClears = %+v, want the rate limit to be cleared", repo.observedClears)
		}
		if repo.state.OverloadUntil == nil {
			t.Fatal("529 overload cooldown must survive the 429 self-heal")
		}
		if repo.state.IsSchedulable() {
			t.Fatal("an account still inside its 529 overload cooldown must not become schedulable")
		}
	})

	// P0-1：probe 最长飞 15s，期间真实请求可能写入一条正确的长封锁。
	// 内存快照是 probe 之前的，条件清除必须因代际不匹配而放弃。
	t.Run("skips when the rate limit generation changed during the probe", func(t *testing.T) {
		account, repo := newLimitedAccount(11, PlatformOpenAI)
		observedLimitedAt, observedResetAt := account.RateLimitedAt, account.RateLimitResetAt

		// probe 飞行期间：真实 429 写入了一条新的、更长的封锁。
		newLimitedAt := time.Now().UTC().Truncate(time.Second)
		newResetAt := newLimitedAt.Add(72 * time.Hour)
		repo.state.RateLimitedAt = &newLimitedAt
		repo.state.RateLimitResetAt = &newResetAt

		svc := &AccountUsageService{accountRepo: repo}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), observedLimitedAt, observedResetAt)

		if len(repo.observedClears) != 1 {
			t.Fatalf("observedClears = %+v, want exactly one attempt", repo.observedClears)
		}
		if repo.clearedCount() != 0 {
			t.Fatal("a rate limit written while the probe was in flight must not be erased")
		}
		if len(repo.unconditionalClears) != 0 {
			t.Fatalf("unconditionalClears = %v, must never fall back to the unconditional clear", repo.unconditionalClears)
		}
		if repo.state.RateLimitResetAt == nil || !repo.state.RateLimitResetAt.Equal(newResetAt) {
			t.Fatal("the newer rate limit must stay intact")
		}
	})

	// P1-3：额度耗尽信号可能只出现在 body（usage_limit_reached），5h/7d header 仍 <100%。
	// handle429 给这类封锁打了来源标记，自愈必须跳过，否则"封锁→刷新解封→立刻再撞 429"。
	t.Run("keeps a quota-derived usage_limit_reached block", func(t *testing.T) {
		account, repo := newLimitedAccount(12, PlatformOpenAI)
		account.Extra = map[string]any{
			openAIRateLimitSourceExtraKey:        openAIRateLimitSourceBodyUsageLimit,
			openAIRateLimitSourceResetAtExtraKey: future.Format(time.RFC3339),
		}

		svc := &AccountUsageService{accountRepo: repo}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if len(repo.observedClears) != 0 || len(repo.unconditionalClears) != 0 {
			t.Fatalf("a body-derived quota block must not be cleared by a healthy codex window snapshot (observed=%+v unconditional=%v)",
				repo.observedClears, repo.unconditionalClears)
		}
		if !account.IsRateLimited() {
			t.Fatal("account must stay rate limited")
		}
	})

	// 陈旧标记不得长期挡住本该发生的自愈：标记绑定的是写入时的 reset_at 代际。
	t.Run("ignores a stale source marker from an older generation", func(t *testing.T) {
		account, repo := newLimitedAccount(13, PlatformOpenAI)
		account.Extra = map[string]any{
			openAIRateLimitSourceExtraKey:        openAIRateLimitSourceBodyUsageLimit,
			openAIRateLimitSourceResetAtExtraKey: future.Add(-48 * time.Hour).Format(time.RFC3339),
		}

		svc := &AccountUsageService{accountRepo: repo}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if repo.clearedCount() != 1 {
			t.Fatalf("observedClears = %+v, a marker from an older generation must not block the heal", repo.observedClears)
		}
		// 标记必须随限流一起被抹掉。
		found := false
		for _, u := range repo.extraUpdates {
			if v, ok := u[openAIRateLimitSourceExtraKey]; ok && v == nil {
				found = true
			}
		}
		if !found {
			t.Fatalf("extraUpdates = %+v, the source marker must be cleared together with the rate limit", repo.extraUpdates)
		}
	})

	t.Run("keeps rate limit when a window is exhausted", func(t *testing.T) {
		account, repo := newLimitedAccount(2, PlatformOpenAI)
		svc := &AccountUsageService{accountRepo: repo}

		exhausted := &UsageInfo{
			FiveHour: &UsageProgress{Utilization: 8},
			SevenDay: &UsageProgress{Utilization: 100},
		}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, exhausted,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if len(repo.observedClears) != 0 || len(repo.unconditionalClears) != 0 {
			t.Fatalf("nothing must be cleared when a plan window is exhausted (observed=%+v unconditional=%v)",
				repo.observedClears, repo.unconditionalClears)
		}
		if !account.IsRateLimited() {
			t.Fatal("account should stay rate limited")
		}
	})

	// 纵深防御：真实路径里影子走 getOpenAIUsage 的另一条分支（见
	// TestGetOpenAIUsage_SparkShadowRateLimitedIsNeverSelfHealed），此处只锁函数自身的守卫。
	t.Run("skips spark shadow accounts", func(t *testing.T) {
		account, repo := newLimitedAccount(3, PlatformOpenAI)
		account.ParentAccountID = &parentID
		account.QuotaDimension = QuotaDimensionSpark
		svc := &AccountUsageService{accountRepo: repo}

		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if len(repo.observedClears) != 0 || len(repo.unconditionalClears) != 0 {
			t.Fatalf("spark shadow must not be healed by global codex signals (observed=%+v unconditional=%v)",
				repo.observedClears, repo.unconditionalClears)
		}
		if !account.IsRateLimited() {
			t.Fatal("spark shadow should stay rate limited")
		}
	})

	t.Run("skips non-openai platforms", func(t *testing.T) {
		account, repo := newLimitedAccount(4, PlatformAnthropic)
		svc := &AccountUsageService{accountRepo: repo}

		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if len(repo.observedClears) != 0 || len(repo.unconditionalClears) != 0 {
			t.Fatalf("nothing must be cleared for non-OpenAI platforms (observed=%+v unconditional=%v)",
				repo.observedClears, repo.unconditionalClears)
		}
	})

	t.Run("skips when window data is incomplete", func(t *testing.T) {
		account, repo := newLimitedAccount(5, PlatformOpenAI)
		svc := &AccountUsageService{accountRepo: repo}

		partial := &UsageInfo{FiveHour: &UsageProgress{Utilization: 3}}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, partial,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if len(repo.observedClears) != 0 || len(repo.unconditionalClears) != 0 {
			t.Fatalf("nothing must be cleared when 7d data is missing (observed=%+v unconditional=%v)",
				repo.observedClears, repo.unconditionalClears)
		}
	})

	// T-3：mergeAccountExtra 是 merge 不是 replace。本轮 probe 只回了一个窗口时，
	// 另一个窗口会沿用 Extra 里的旧值，而"窗口已过期 ⇒ Utilization 归零"会把它伪装成健康的 0%。
	t.Run("skips when this round's snapshot is missing a window", func(t *testing.T) {
		account, repo := newLimitedAccount(6, PlatformOpenAI)
		svc := &AccountUsageService{accountRepo: repo}

		onlySevenDay := map[string]any{"codex_7d_used_percent": 40.0}
		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			onlySevenDay, account.RateLimitedAt, account.RateLimitResetAt)

		if len(repo.observedClears) != 0 || len(repo.unconditionalClears) != 0 {
			t.Fatalf("a snapshot missing codex_5h_used_percent must not be trusted (observed=%+v unconditional=%v)",
				repo.observedClears, repo.unconditionalClears)
		}
	})

	t.Run("keeps the in-memory rate limit when the clear fails", func(t *testing.T) {
		account, repo := newLimitedAccount(8, PlatformOpenAI)
		repo.observedErr = errors.New("db down")
		svc := &AccountUsageService{accountRepo: repo}

		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if !account.IsRateLimited() {
			t.Fatal("a failed clear must not optimistically clear the in-memory snapshot")
		}
		if len(repo.extraUpdates) != 0 {
			t.Fatalf("extraUpdates = %+v, the source marker must not be cleared when the rate limit was not", repo.extraUpdates)
		}
	})

	t.Run("skips when the repository has no conditional clear primitive", func(t *testing.T) {
		account, _ := newLimitedAccount(7, PlatformOpenAI)
		bare := &noConditionalClearRepo{}
		svc := &AccountUsageService{accountRepo: bare}

		svc.clearOpenAIRateLimitIfCodexSnapshotHealthy(context.Background(), account, healthy,
			healthyCodexUpdates(), account.RateLimitedAt, account.RateLimitResetAt)

		if len(bare.clearedIDs) != 0 {
			t.Fatalf("clearedIDs = %v, must never fall back to the unconditional ClearRateLimit", bare.clearedIDs)
		}
		if !account.IsRateLimited() {
			t.Fatal("account must stay rate limited when no safe primitive is available")
		}
	})
}

// noConditionalClearRepo 只实现无条件 ClearRateLimit，用于锁住"缺少条件清除原语时宁可不清"。
type noConditionalClearRepo struct {
	stubOpenAIAccountRepo
	clearedIDs []int64
}

func (r *noConditionalClearRepo) ClearRateLimit(_ context.Context, id int64) error {
	r.clearedIDs = append(r.clearedIDs, id)
	return nil
}
