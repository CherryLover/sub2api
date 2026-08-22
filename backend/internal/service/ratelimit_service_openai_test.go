//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalculateOpenAI429ResetTime_7dExhausted(t *testing.T) {
	svc := &RateLimitService{}

	// Simulate headers when 7d limit is exhausted (100% used)
	// Primary = 7d (10080 minutes), Secondary = 5h (300 minutes)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "384607") // ~4.5 days
	headers.Set("x-codex-primary-window-minutes", "10080")       // 7 days
	headers.Set("x-codex-secondary-used-percent", "3")
	headers.Set("x-codex-secondary-reset-after-seconds", "17369") // ~4.8 hours
	headers.Set("x-codex-secondary-window-minutes", "300")        // 5 hours

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should be approximately 384607 seconds from now
	expectedDuration := 384607 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

func TestCalculateOpenAI429ResetTime_5hExhausted(t *testing.T) {
	svc := &RateLimitService{}

	// Simulate headers when 5h limit is exhausted (100% used)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "50")
	headers.Set("x-codex-primary-reset-after-seconds", "500000")
	headers.Set("x-codex-primary-window-minutes", "10080") // 7 days
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "3600") // 1 hour
	headers.Set("x-codex-secondary-window-minutes", "300")       // 5 hours

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should be approximately 3600 seconds from now
	expectedDuration := 3600 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

// TestCalculateOpenAI429ResetTime_NeitherExhausted_ReturnsNil 锁住本次修复的核心语义：
// 两个套餐窗口都没耗尽时收到的 429 不是额度问题（多为瞬时/并发限流或上游代理 429），
// 绝不能按"较长的那个窗口 reset"（几乎总是 7d）封锁账号——那正是账号被误挂几天 429 的根因。
// 此处必须返回 nil，把决定权交回调用方的秒级兜底冷却。
func TestCalculateOpenAI429ResetTime_NeitherExhausted_ReturnsNil(t *testing.T) {
	svc := &RateLimitService{}

	// Neither window is exhausted (<100%), even though both carry long reset headers.
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "80")
	headers.Set("x-codex-primary-reset-after-seconds", "100000")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "90")
	headers.Set("x-codex-secondary-reset-after-seconds", "5000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	resetAt := svc.calculateOpenAI429ResetTime(headers)

	require.Nil(t, resetAt, "neither window exhausted must not produce a window-based block time")
}

// TestCalculateOpenAI429ResetTime_Only5hExhausted 仅 5h 耗尽 → 用 5h 自己的 reset。
func TestCalculateOpenAI429ResetTime_Only5hExhausted(t *testing.T) {
	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "42")
	headers.Set("x-codex-primary-reset-after-seconds", "400000")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "1800")
	headers.Set("x-codex-secondary-window-minutes", "300")

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	require.NotNil(t, resetAt)
	require.False(t, resetAt.Before(before.Add(1800*time.Second)))
	require.False(t, resetAt.After(after.Add(1800*time.Second)))
}

// TestCalculateOpenAI429ResetTime_Only7dExhausted 仅 7d 耗尽 → 用 7d 自己的 reset。
func TestCalculateOpenAI429ResetTime_Only7dExhausted(t *testing.T) {
	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "200000")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "5")
	headers.Set("x-codex-secondary-reset-after-seconds", "900")
	headers.Set("x-codex-secondary-window-minutes", "300")

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	require.NotNil(t, resetAt)
	require.False(t, resetAt.Before(before.Add(200000*time.Second)))
	require.False(t, resetAt.After(after.Add(200000*time.Second)))
}

// TestCalculateOpenAI429ResetTime_BothExhausted_Prefers7d 两个窗口都耗尽时 7d 优先，
// 因为 7d 耗尽实际恢复得更晚，按 5h 解封会立刻再撞 429。
func TestCalculateOpenAI429ResetTime_BothExhausted_Prefers7d(t *testing.T) {
	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "300000")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "1200")
	headers.Set("x-codex-secondary-window-minutes", "300")

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	require.NotNil(t, resetAt)
	require.False(t, resetAt.Before(before.Add(300000*time.Second)))
	require.False(t, resetAt.After(after.Add(300000*time.Second)))
}

// TestCalculateOpenAI429ResetTime_ExhaustedWithoutOwnReset_FallsBackToOtherExhaustedWindow
// 7d 耗尽但缺自己的 reset 头，而 5h 同样耗尽且有 reset → 用 5h 的，而不是无限期封锁。
func TestCalculateOpenAI429ResetTime_ExhaustedWithoutOwnReset_FallsBackToOtherExhaustedWindow(t *testing.T) {
	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-window-minutes", "10080") // 7d exhausted, no reset header
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "600")
	headers.Set("x-codex-secondary-window-minutes", "300")

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	require.NotNil(t, resetAt)
	require.False(t, resetAt.Before(before.Add(600*time.Second)))
	require.False(t, resetAt.After(after.Add(600*time.Second)))
}

// TestCalculateOpenAI429ResetTime_ExhaustedWithoutAnyReset_ReturnsNil
// 窗口耗尽但没有任何可用的 reset 头：不拿另一个未耗尽窗口的 reset 去猜长封锁时间，返回 nil。
func TestCalculateOpenAI429ResetTime_ExhaustedWithoutAnyReset_ReturnsNil(t *testing.T) {
	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-window-minutes", "10080") // 7d exhausted, no reset header
	headers.Set("x-codex-secondary-used-percent", "10")
	headers.Set("x-codex-secondary-reset-after-seconds", "50000") // 未耗尽窗口的 reset 不得被借用
	headers.Set("x-codex-secondary-window-minutes", "300")

	resetAt := svc.calculateOpenAI429ResetTime(headers)

	require.Nil(t, resetAt, "must not borrow a non-exhausted window's reset time")
}

func TestCalculateOpenAI429ResetTime_NoCodexHeaders(t *testing.T) {
	svc := &RateLimitService{}

	// No codex headers at all
	headers := http.Header{}
	headers.Set("content-type", "application/json")

	resetAt := svc.calculateOpenAI429ResetTime(headers)

	if resetAt != nil {
		t.Errorf("expected nil resetAt when no codex headers, got %v", resetAt)
	}
}

func TestCalculateOpenAI429ResetTime_ReversedWindowOrder(t *testing.T) {
	svc := &RateLimitService{}

	// Test when OpenAI sends primary as 5h and secondary as 7d (reversed)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")         // This is 5h
	headers.Set("x-codex-primary-reset-after-seconds", "3600") // 1 hour
	headers.Set("x-codex-primary-window-minutes", "300")       // 5 hours - smaller!
	headers.Set("x-codex-secondary-used-percent", "50")
	headers.Set("x-codex-secondary-reset-after-seconds", "500000")
	headers.Set("x-codex-secondary-window-minutes", "10080") // 7 days - larger!

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should correctly identify that primary is 5h (smaller window) and use its reset time
	expectedDuration := 3600 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

type openAI429SnapshotRepo struct {
	mockAccountRepoForGemini
	rateLimitedID      int64
	rateLimitResetAt   time.Time
	updatedExtra       map[string]any
	bulkUpdatedIDs     []int64
	bulkUpdatedPayload AccountBulkUpdate
}

func (r *openAI429SnapshotRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedID = id
	r.rateLimitResetAt = resetAt
	return nil
}

// UpdateExtra 按真实实现的 jsonb `||` 语义做累积合并，而不是整体替换——
// 一次 429 现在可能触发多次 Extra 写入（codex 快照 + 限流来源标记）。
func (r *openAI429SnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updatedExtra == nil {
		r.updatedExtra = make(map[string]any, len(updates))
	}
	for k, v := range updates {
		r.updatedExtra[k] = v
	}
	return nil
}

func (r *openAI429SnapshotRepo) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.bulkUpdatedIDs = append([]int64(nil), ids...)
	r.bulkUpdatedPayload = updates
	return int64(len(ids)), nil
}

func TestHandle429_OpenAIPersistsCodexSnapshotImmediately(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	svc.handle429(context.Background(), account, headers, nil)

	if repo.rateLimitedID != account.ID {
		t.Fatalf("rateLimitedID = %d, want %d", repo.rateLimitedID, account.ID)
	}
	if len(repo.updatedExtra) == 0 {
		t.Fatal("expected codex snapshot to be persisted on 429")
	}
	if got := repo.updatedExtra["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := repo.updatedExtra["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

// TestHandle429_OpenAINeitherWindowExhausted_UsesSecondsFallback 端到端锁住本次修复的用户可见价值：
// OpenAI 账号收到一个"两个套餐窗口都没耗尽、body 里也没有 resets_* 字段"的 429（典型的瞬时/并发
// 限流或上游代理 429）时，最终只应落到管理端可配的秒级兜底冷却，而不是被按 7d 窗口封几天。
func TestHandle429_OpenAINeitherWindowExhausted_UsesSecondsFallback(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 777, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "12")
	headers.Set("x-codex-primary-reset-after-seconds", "500000") // 7d 窗口滚动还剩 ~5.8 天
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "31")
	headers.Set("x-codex-secondary-reset-after-seconds", "9000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	// 瞬时限流的典型 body：没有 resets_at / resets_in_seconds。
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached, please slow down"}}`)

	before := time.Now()
	svc.handle429(context.Background(), account, headers, body)

	require.Equal(t, account.ID, repo.rateLimitedID, "account should still get a short cooldown so failover budget is not burned")
	require.False(t, repo.rateLimitResetAt.IsZero())

	cooldown := repo.rateLimitResetAt.Sub(before)
	require.Greater(t, cooldown, time.Duration(0))
	require.LessOrEqual(t, cooldown, time.Duration(defaultRateLimit429CooldownSeconds)*time.Second+2*time.Second,
		"must be the seconds-level 429 fallback, not a window-length block")
	require.Less(t, cooldown, time.Minute, "must never be hours/days for a non-exhaustion 429")

	// 快照仍应被写入，方便管理页看到真实用量。
	require.Equal(t, 12.0, repo.updatedExtra["codex_7d_used_percent"])
	require.Equal(t, 31.0, repo.updatedExtra["codex_5h_used_percent"])
}

func TestHandle429_OpenAISyncsObservedPlanType(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{
		ID:          124,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "plus"},
	}
	body := []byte(`{"error":{"type":"usage_limit_reached","message":"limit reached","plan_type":"free","resets_at":1777283883}}`)

	svc.handle429(context.Background(), account, http.Header{}, body)

	require.Equal(t, []int64{account.ID}, repo.bulkUpdatedIDs)
	require.Equal(t, "free", repo.bulkUpdatedPayload.Credentials["plan_type"])
	require.Equal(t, "free", account.Credentials["plan_type"])
	require.Equal(t, account.ID, repo.rateLimitedID)
}

// TestHandle429_SkipsSparkShadow 外审第8轮 P1:spark 影子的限流状态只由 QueryUsage(/wham/usage
// codex_bengalfox)维护;/responses 429 携带的 global x-codex-* 不得对影子做任何 DB 限流写入,
// 否则会把 spark 误耦合到 global codex 窗口、冷却到 global reset。
func TestHandle429_SkipsSparkShadow(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	parentID := int64(900)
	shadowRepo := &openAI429SnapshotRepo{}
	shadowSvc := NewRateLimitService(shadowRepo, nil, nil, nil, nil)
	shadow := &Account{
		ID:              901,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
	}

	shadowSvc.handle429(context.Background(), shadow, headers, nil)

	require.Zero(t, shadowRepo.rateLimitedID, "spark shadow must not be SetRateLimited from /responses global 429")
	require.Empty(t, shadowRepo.updatedExtra, "spark shadow must not get a codex snapshot from /responses 429")

	// 反向对照:普通 OpenAI OAuth 账号仍按 global 429 限流。
	normalRepo := &openAI429SnapshotRepo{}
	normalSvc := NewRateLimitService(normalRepo, nil, nil, nil, nil)
	normal := &Account{ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	normalSvc.handle429(context.Background(), normal, headers, nil)

	require.Equal(t, normal.ID, normalRepo.rateLimitedID, "normal OpenAI OAuth account should still be rate limited")
}

func TestNormalizedCodexLimits(t *testing.T) {
	// Test the Normalize() method directly
	pUsed := 100.0
	pReset := 384607
	pWindow := 10080
	sUsed := 3.0
	sReset := 17369
	sWindow := 300

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &pUsed,
		PrimaryResetAfterSeconds:   &pReset,
		PrimaryWindowMinutes:       &pWindow,
		SecondaryUsedPercent:       &sUsed,
		SecondaryResetAfterSeconds: &sReset,
		SecondaryWindowMinutes:     &sWindow,
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Primary has larger window (10080 > 300), so primary should be 7d
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 100.0 {
		t.Errorf("expected Used7dPercent=100, got %v", normalized.Used7dPercent)
	}
	if normalized.Reset7dSeconds == nil || *normalized.Reset7dSeconds != 384607 {
		t.Errorf("expected Reset7dSeconds=384607, got %v", normalized.Reset7dSeconds)
	}
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 3.0 {
		t.Errorf("expected Used5hPercent=3, got %v", normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds == nil || *normalized.Reset5hSeconds != 17369 {
		t.Errorf("expected Reset5hSeconds=17369, got %v", normalized.Reset5hSeconds)
	}
}

func TestNormalizedCodexLimits_OnlyPrimaryData(t *testing.T) {
	// Test when only primary has data, no window_minutes
	pUsed := 80.0
	pReset := 50000

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:       &pUsed,
		PrimaryResetAfterSeconds: &pReset,
		// No window_minutes, no secondary data
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Legacy assumption: primary=7d, secondary=5h
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 80.0 {
		t.Errorf("expected Used7dPercent=80, got %v", normalized.Used7dPercent)
	}
	if normalized.Reset7dSeconds == nil || *normalized.Reset7dSeconds != 50000 {
		t.Errorf("expected Reset7dSeconds=50000, got %v", normalized.Reset7dSeconds)
	}
	// Secondary (5h) should be nil
	if normalized.Used5hPercent != nil {
		t.Errorf("expected Used5hPercent=nil, got %v", *normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds != nil {
		t.Errorf("expected Reset5hSeconds=nil, got %v", *normalized.Reset5hSeconds)
	}
}

func TestRateLimitService_HandleUpstreamError_403PreservesOriginalUpstreamMessage(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       201,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		403,
		http.Header{},
		[]byte(`{"error":{"message":"workspace forbidden by policy","type":"invalid_request_error"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Contains(t, repo.lastErrorMsg, "workspace forbidden by policy")
	require.NotContains(t, repo.lastErrorMsg, "account may be suspended or lack permissions")
}

func TestRateLimitService_HandleUpstreamError_403FallsBackToRawBody(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       202,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		403,
		http.Header{},
		[]byte(`{"error":{"type":"access_denied","details":{"reason":"ip_blocked"}}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Contains(t, repo.lastErrorMsg, `"access_denied"`)
	require.Contains(t, repo.lastErrorMsg, `"ip_blocked"`)
	require.NotContains(t, repo.lastErrorMsg, "account may be suspended or lack permissions")
}

func TestNormalizedCodexLimits_OnlySecondaryData(t *testing.T) {
	// Test when only secondary has data, no window_minutes
	sUsed := 60.0
	sReset := 3000

	snapshot := &OpenAICodexUsageSnapshot{
		SecondaryUsedPercent:       &sUsed,
		SecondaryResetAfterSeconds: &sReset,
		// No window_minutes, no primary data
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Legacy assumption: primary=7d, secondary=5h
	// So secondary goes to 5h
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 60.0 {
		t.Errorf("expected Used5hPercent=60, got %v", normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds == nil || *normalized.Reset5hSeconds != 3000 {
		t.Errorf("expected Reset5hSeconds=3000, got %v", normalized.Reset5hSeconds)
	}
	// Primary (7d) should be nil
	if normalized.Used7dPercent != nil {
		t.Errorf("expected Used7dPercent=nil, got %v", *normalized.Used7dPercent)
	}
}

func TestNormalizedCodexLimits_BothDataNoWindowMinutes(t *testing.T) {
	// Test when both have data but no window_minutes
	pUsed := 100.0
	pReset := 400000
	sUsed := 50.0
	sReset := 10000

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &pUsed,
		PrimaryResetAfterSeconds:   &pReset,
		SecondaryUsedPercent:       &sUsed,
		SecondaryResetAfterSeconds: &sReset,
		// No window_minutes
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Legacy assumption: primary=7d, secondary=5h
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 100.0 {
		t.Errorf("expected Used7dPercent=100, got %v", normalized.Used7dPercent)
	}
	if normalized.Reset7dSeconds == nil || *normalized.Reset7dSeconds != 400000 {
		t.Errorf("expected Reset7dSeconds=400000, got %v", normalized.Reset7dSeconds)
	}
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 50.0 {
		t.Errorf("expected Used5hPercent=50, got %v", normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds == nil || *normalized.Reset5hSeconds != 10000 {
		t.Errorf("expected Reset5hSeconds=10000, got %v", normalized.Reset5hSeconds)
	}
}

func TestHandle429_AnthropicPlatformUnaffected(t *testing.T) {
	// Verify that Anthropic platform accounts still use the original logic
	// This test ensures we don't break existing Claude account rate limiting

	svc := &RateLimitService{}

	// Simulate Anthropic 429 headers
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-reset", "1737820800") // A future Unix timestamp

	// For Anthropic platform, calculateOpenAI429ResetTime should return nil
	// because it only handles OpenAI platform
	resetAt := svc.calculateOpenAI429ResetTime(headers)

	// Should return nil since there are no x-codex-* headers
	if resetAt != nil {
		t.Errorf("expected nil for Anthropic headers, got %v", resetAt)
	}
}

func TestCalculateOpenAI429ResetTime_UserProvidedScenario(t *testing.T) {
	// This is the exact scenario from the user:
	// codex_7d_used_percent: 100
	// codex_7d_reset_after_seconds: 384607 (约4.5天后重置)
	// codex_5h_used_percent: 3
	// codex_5h_reset_after_seconds: 17369 (约4.8小时后重置)

	svc := &RateLimitService{}

	// Simulate headers matching user's data
	// Note: We need to map the canonical 5h/7d back to primary/secondary
	// Based on typical OpenAI behavior: primary=7d (larger window), secondary=5h (smaller window)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "384607")
	headers.Set("x-codex-primary-window-minutes", "10080") // 7 days = 10080 minutes
	headers.Set("x-codex-secondary-used-percent", "3")
	headers.Set("x-codex-secondary-reset-after-seconds", "17369")
	headers.Set("x-codex-secondary-window-minutes", "300") // 5 hours = 300 minutes

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt for user scenario")
	}

	// Should use the 7d reset time (384607 seconds) since 7d limit is exhausted (100%)
	expectedDuration := 384607 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}

	// Verify it's approximately 4.45 days (384607 seconds)
	duration := resetAt.Sub(before)
	actualDays := duration.Hours() / 24.0

	// 384607 / 86400 = ~4.45 days
	if actualDays < 4.4 || actualDays > 4.5 {
		t.Errorf("expected ~4.45 days, got %.2f days", actualDays)
	}

	t.Logf("User scenario: reset_at=%v, duration=%.2f days", resetAt, actualDays)
}

func TestCalculateOpenAI429ResetTime_5MinFallbackWhenNoReset(t *testing.T) {
	// Test that we return nil when there's used_percent but no reset_after_seconds
	// This should cause the caller to use the default 5-minute fallback

	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	// No reset_after_seconds!

	resetAt := svc.calculateOpenAI429ResetTime(headers)

	// Should return nil since there's no reset time available
	if resetAt != nil {
		t.Errorf("expected nil when no reset_after_seconds, got %v", resetAt)
	}
}

// TestHandle429_OpenAIBodyUsageLimitMarksQuotaDerivedBlock 锁住改动 A 与改动 B 的交界：
//
// 改动 A 刻意保留了"额度耗尽信号只在 body、不在 5h/7d header"这条通路——两个窗口都 <100%
// 时 calculateOpenAI429ResetTime 返回 nil，handle429 降级到 body 的 usage_limit_reached
// 并按 resets_at 正确封锁。改动 B 的自愈判据恰恰是"两个窗口都 <100% ⇒ 解封"，若不加区分
// 就会把 A 保留的这条正确封锁推翻。
//
// 因此 handle429 必须给 body 派生的封锁打上来源标记，且标记要绑定到本次的 reset_at 代际。
func TestHandle429_OpenAIBodyUsageLimitMarksQuotaDerivedBlock(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 4711, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	// header 口径：两个窗口都远未耗尽（上游代理改写头 / plan 级 credit 限制的典型形态）。
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "9")
	headers.Set("x-codex-primary-reset-after-seconds", "500000")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "21")
	headers.Set("x-codex-secondary-reset-after-seconds", "9000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	resetsAt := time.Now().Add(50 * time.Hour).Unix()
	body := []byte(fmt.Sprintf(`{"error":{"type":"usage_limit_reached","message":"You've hit your usage limit.","resets_at":%d}}`, resetsAt))

	svc.handle429(context.Background(), account, headers, body)

	require.Equal(t, account.ID, repo.rateLimitedID)
	require.Equal(t, resetsAt, repo.rateLimitResetAt.Unix(),
		"body-derived usage_limit_reached must still block until resets_at (change A's preserved path)")

	require.Equal(t, openAIRateLimitSourceBodyUsageLimit, repo.updatedExtra[openAIRateLimitSourceExtraKey],
		"a quota-derived block must be marked so the codex snapshot self-heal skips it")
	require.Equal(t, time.Unix(resetsAt, 0).UTC().Format(time.RFC3339), repo.updatedExtra[openAIRateLimitSourceResetAtExtraKey],
		"the marker must be pinned to the generation it describes")
	// 内存快照也要跟上，否则同一请求链后续读到的还是旧值。
	require.Equal(t, openAIRateLimitSourceBodyUsageLimit, account.Extra[openAIRateLimitSourceExtraKey])

	// 端到端：把刚写下的这条限流装回账号，自愈判据必须认出它是额度派生的。
	blocked := &Account{
		ID:               account.ID,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		RateLimitedAt:    &repo.rateLimitResetAt,
		RateLimitResetAt: &repo.rateLimitResetAt,
		Extra:            account.Extra,
	}
	require.True(t, isOpenAIQuotaDerivedRateLimit(blocked))
}

// TestHandle429_OpenAIWindowExhaustedIsNotMarkedQuotaDerived 反向对照：
// 由 5h/7d 窗口耗尽写下的封锁不打标记——它本来就该在窗口滚动后被 codex 快照自愈清掉。
func TestHandle429_OpenAIWindowExhaustedIsNotMarkedQuotaDerived(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 4712, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "40")
	headers.Set("x-codex-secondary-reset-after-seconds", "9000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	svc.handle429(context.Background(), account, headers, []byte(`{"error":{"type":"usage_limit_reached","resets_at":1}}`))

	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotContains(t, repo.updatedExtra, openAIRateLimitSourceExtraKey,
		"a window-derived block must stay healable by the codex snapshot self-heal")
}

// TestOpenAIRateLimitSourceFor_GenerationBinding 锁住"陈旧标记不得误导后续判断"：
// 标记只有在它记录的 reset_at 与账号当前的 rate_limit_reset_at 属于同一代际时才算数。
func TestOpenAIRateLimitSourceFor_GenerationBinding(t *testing.T) {
	resetAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	newAccount := func(markerResetAt string) *Account {
		reset := resetAt
		return &Account{
			ID:               5001,
			Platform:         PlatformOpenAI,
			Type:             AccountTypeOAuth,
			RateLimitResetAt: &reset,
			Extra: map[string]any{
				openAIRateLimitSourceExtraKey:        openAIRateLimitSourceBodyUsageLimit,
				openAIRateLimitSourceResetAtExtraKey: markerResetAt,
			},
		}
	}

	require.Equal(t, openAIRateLimitSourceBodyUsageLimit,
		openAIRateLimitSourceFor(newAccount(resetAt.Format(time.RFC3339))),
		"marker for the current generation must be honoured")

	require.Empty(t, openAIRateLimitSourceFor(newAccount(resetAt.Add(-time.Hour).Format(time.RFC3339))),
		"a marker left over from an older generation must be ignored")

	require.Empty(t, openAIRateLimitSourceFor(newAccount("")),
		"an unparsable marker timestamp must be ignored")

	// 限流已被清除：即便 Extra 里还留着标记也不得生效。
	noLimit := newAccount(resetAt.Format(time.RFC3339))
	noLimit.RateLimitResetAt = nil
	require.Empty(t, openAIRateLimitSourceFor(noLimit))

	// 清除动作写下的 null 值必须被读作"无标记"。
	cleared := newAccount(resetAt.Format(time.RFC3339))
	for k, v := range openAIRateLimitSourceClearUpdates() {
		cleared.Extra[k] = v
	}
	require.Empty(t, openAIRateLimitSourceFor(cleared))
}
