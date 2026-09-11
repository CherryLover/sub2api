//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 这组用例锁住 2026-09-10 线上故障的修复：
// 上游的额度是按「池」组织的（主池 rate_limit + 附加池 codex_bengalfox），
// 一个附加池打满绝不能被当成整号额度用尽。

type dedicatedQuotaPoolModelRateLimitCall struct {
	accountID int64
	scope     string
	resetAt   time.Time
	reason    string
}

type dedicatedQuotaPoolRepo struct {
	mockAccountRepoForGemini

	setRateLimitedCalls int
	rateLimitedID       int64
	rateLimitedResetAt  time.Time

	modelRateLimitCalls []dedicatedQuotaPoolModelRateLimitCall
	modelRateLimitErr   error

	sessionWindowCalls int
	tempUnschedCalls   int
	extraUpdates       map[string]any
}

func (r *dedicatedQuotaPoolRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.setRateLimitedCalls++
	r.rateLimitedID = id
	r.rateLimitedResetAt = resetAt
	return nil
}

func (r *dedicatedQuotaPoolRepo) SetModelRateLimit(_ context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	call := dedicatedQuotaPoolModelRateLimitCall{accountID: id, scope: scope, resetAt: resetAt}
	if len(reason) > 0 {
		call.reason = reason[0]
	}
	r.modelRateLimitCalls = append(r.modelRateLimitCalls, call)
	return r.modelRateLimitErr
}

func (r *dedicatedQuotaPoolRepo) UpdateSessionWindow(_ context.Context, _ int64, _, _ *time.Time, _ string) error {
	r.sessionWindowCalls++
	return nil
}

func (r *dedicatedQuotaPoolRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

func (r *dedicatedQuotaPoolRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.extraUpdates == nil {
		r.extraUpdates = make(map[string]any, len(updates))
	}
	for k, v := range updates {
		r.extraUpdates[k] = v
	}
	return nil
}

// mainPoolHealthyCodexHeaders 复刻公司实例账号 7 在 2026-09-10 的真实状态：
// x-codex-* 描述的是**主池**，主池 7d 只用了 1%、5h 几乎没用，两个窗口都没耗尽。
func mainPoolHealthyCodexHeaders() http.Header {
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "1") // 7d 主池
	headers.Set("x-codex-primary-reset-after-seconds", "672720")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "0") // 5h
	headers.Set("x-codex-secondary-reset-after-seconds", "17000")
	headers.Set("x-codex-secondary-window-minutes", "300")
	return headers
}

func sparkPoolExhaustedBody(resetAt time.Time) []byte {
	return []byte(fmt.Sprintf(
		`{"error":{"type":"usage_limit_reached","message":"You've hit your usage limit.","resets_at":%d}}`,
		resetAt.Unix()))
}

// 线上原案复现：账号主池 7d 仅 1%、spark 池 7d 100%，429 的 reset 指向 spark 池的重置时间。
// 期望：只写该模型的 model_rate_limits，accounts.rate_limit_reset_at 必须保持 NULL。
func TestHandleUpstreamError_SparkPoolExhaustedWritesModelScopeOnly(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	sparkPoolReset := time.Now().Add(4*24*time.Hour + 3*time.Hour)

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		mainPoolHealthyCodexHeaders(),
		sparkPoolExhaustedBody(sparkPoolReset),
		"gpt-5.3-codex-spark",
	)

	require.False(t, shouldDisable)
	require.Zero(t, repo.setRateLimitedCalls,
		"spark 池耗尽绝不能写 accounts.rate_limit_reset_at —— 这正是 2026-09-10 11:01:59 做错的那一次")
	require.Nil(t, account.RateLimitResetAt)
	require.Zero(t, repo.sessionWindowCalls, "模型级限流不应改写账号的 5h session window")
	require.Zero(t, repo.tempUnschedCalls, "模型级限流不应把整号标记为临时不可调度")

	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, int64(7), call.accountID)
	require.Equal(t, "gpt-5.3-codex-spark", call.scope)
	require.Equal(t, openAIDedicatedQuotaPoolRateLimitReason, call.reason)
	require.WithinDuration(t, sparkPoolReset, call.resetAt, time.Second,
		"冷却截止时间应取上游给的 spark 池重置时间")
}

// 写进 model_rate_limits 之后，同账号的其它模型（主池）必须仍然可调度。
func TestSparkPoolModelRateLimitKeepsOtherModelsSchedulable(t *testing.T) {
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	setAccountModelRateLimitSnapshot(account, "gpt-5.3-codex-spark",
		time.Now().Add(4*24*time.Hour), openAIDedicatedQuotaPoolRateLimitReason, time.Now())

	ctx := context.Background()
	require.True(t, isAccountModelRateLimitedForScheduling(ctx, account, "gpt-5.3-codex-spark"))
	for _, model := range []string{"gpt-6-astra", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra"} {
		require.False(t, isAccountModelRateLimitedForScheduling(ctx, account, model),
			"主池模型 %s 不应被 spark 池的限流牵连", model)
		require.True(t, account.IsSchedulableForModelWithContext(ctx, model))
	}
	require.False(t, account.IsSchedulableForModelWithContext(ctx, "gpt-5.3-codex-spark"))
	require.Nil(t, account.RateLimitResetAt, "账号级限流字段必须保持为空")
}

// 普通模型（主池）的 429 行为完全不变：仍然整号限流。
func TestHandleUpstreamError_MainPoolModel429StillRateLimitsWholeAccount(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100") // 主池 7d 真的耗尽
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	for _, model := range []string{"gpt-6-astra", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra"} {
		repo.setRateLimitedCalls = 0
		repo.modelRateLimitCalls = nil

		svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, nil, model)

		require.Equal(t, 1, repo.setRateLimitedCalls, "主池模型 %s 的 429 必须仍然整号限流", model)
		require.Equal(t, int64(11), repo.rateLimitedID)
		require.Empty(t, repo.modelRateLimitCalls, "主池模型不应写模型级限流")
	}
}

// 模型级限流落库失败，绝不允许升级成整号限流。
func TestHandleUpstreamError_SparkPoolPersistFailureDoesNotEscalateToAccount(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{modelRateLimitErr: errors.New("jsonb_set failed")}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		mainPoolHealthyCodexHeaders(),
		sparkPoolExhaustedBody(time.Now().Add(72*time.Hour)),
		"gpt-5.3-codex-spark",
	)

	require.False(t, shouldDisable)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Zero(t, repo.setRateLimitedCalls, "写失败也不能 fall through 到 SetRateLimited")
	require.Zero(t, repo.tempUnschedCalls)
	require.Nil(t, account.RateLimitResetAt)
}

// 模型名只在 ctx 里（withTempUnschedulableModel）时也必须能识别出独立额度池。
func TestHandle429_SparkPoolResolvesModelFromContext(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	ctx := withTempUnschedulableModel(context.Background(), []string{"gpt-5.3-codex-spark"})
	svc.handle429(ctx, account, mainPoolHealthyCodexHeaders(), sparkPoolExhaustedBody(time.Now().Add(48*time.Hour)))

	require.Zero(t, repo.setRateLimitedCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.3-codex-spark", repo.modelRateLimitCalls[0].scope)
}

// 上游没给任何重置时间时，退到管理端可配的秒级 429 兜底冷却，但依旧只写模型级。
func TestHandle429_SparkPoolWithoutResetUsesSecondsFallbackOnModelScope(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	before := time.Now()
	svc.handle429(context.Background(), account, mainPoolHealthyCodexHeaders(),
		[]byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached"}}`),
		"gpt-5.3-codex-spark")

	require.Zero(t, repo.setRateLimitedCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	cooldown := repo.modelRateLimitCalls[0].resetAt.Sub(before)
	require.Greater(t, cooldown, time.Duration(0))
	require.LessOrEqual(t, cooldown, time.Duration(defaultRateLimit429CooldownSeconds)*time.Second+2*time.Second)
}

// Retry-After 优先于响应体，响应体优先于 x-codex-* 主池窗口头。
func TestSparkPool429ResetAtPrefersRetryAfterThenBody(t *testing.T) {
	svc := NewRateLimitService(&dedicatedQuotaPoolRepo{}, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	headers := mainPoolHealthyCodexHeaders()
	headers.Set("Retry-After", "120")
	bodyReset := time.Now().Add(48 * time.Hour)

	resetAt, ok := svc.openAIDedicatedQuotaPool429ResetAt(context.Background(), account, headers, sparkPoolExhaustedBody(bodyReset))
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(120*time.Second), resetAt, 2*time.Second)

	headers.Del("Retry-After")
	resetAt, ok = svc.openAIDedicatedQuotaPool429ResetAt(context.Background(), account, headers, sparkPoolExhaustedBody(bodyReset))
	require.True(t, ok)
	require.WithinDuration(t, bodyReset, resetAt, time.Second)
}

// 账号把 spark 映射到了主池模型时，真正被限流的不是独立池，必须交回整号路径。
func TestHandleUpstreamError_SparkMappedToMainPoolModelFallsBackToAccountLimit(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:          13,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.3-codex-spark": "gpt-5.5"}},
	}

	svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		mainPoolHealthyCodexHeaders(),
		sparkPoolExhaustedBody(time.Now().Add(48*time.Hour)),
		"gpt-5.3-codex-spark",
	)

	require.Empty(t, repo.modelRateLimitCalls)
	require.Equal(t, 1, repo.setRateLimitedCalls)
}

// Spark 影子账号的限流只由 /wham/usage 的 codex_bengalfox 道驱动，429 不得留下任何痕迹。
func TestHandleUpstreamError_SparkPoolSkipsShadowAccount(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	parentID := int64(900)
	shadow := &Account{
		ID:              901,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
	}

	svc.HandleUpstreamError(
		context.Background(),
		shadow,
		http.StatusTooManyRequests,
		mainPoolHealthyCodexHeaders(),
		sparkPoolExhaustedBody(time.Now().Add(48*time.Hour)),
		"gpt-5.3-codex-spark",
	)

	require.Empty(t, repo.modelRateLimitCalls)
	require.Zero(t, repo.setRateLimitedCalls)
}

// 名单判定：归一化要覆盖大小写/空白/下划线/路径前缀/日期与 compact 后缀；
// gpt-5.6-sol 属于主池，绝不能进名单（/wham/usage 只暴露了 codex_bengalfox 一个附加池）。
func TestIsOpenAIDedicatedQuotaPoolModel(t *testing.T) {
	t.Parallel()
	for _, model := range []string{
		"gpt-5.3-codex-spark",
		"  GPT-5.3-Codex-Spark  ",
		"gpt-5.3_codex_spark",
		"openai/gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-2026-01-01",
		"gpt-5.3-codex-spark-openai-compact",
	} {
		require.True(t, IsOpenAIDedicatedQuotaPoolModel(model), "%q should be a dedicated quota pool model", model)
	}
	for _, model := range []string{
		"",
		"gpt-5.6-sol", // 主池：上游只暴露了 codex_bengalfox 一个附加池
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-6-astra",
		"gpt-5.5",
		"gpt-5.3-codex", // 非 spark 的 codex 走主池
		"claude-fable-5",
	} {
		require.False(t, IsOpenAIDedicatedQuotaPoolModel(model), "%q must not be treated as a dedicated quota pool model", model)
	}
}

// 非 OpenAI 平台不适用（scope 解析必须返回空）。
//
// scope 刻意**不做小写归一**：读取侧 modelRateLimitKeysForRequest 查的是
// account.GetMappedModel(requestedModel) 原样（只 TrimSpace，不改大小写），
// 写入侧必须逐字节对上同一个字符串，否则限流写进库里永远没人读到。
// 所以无映射时传什么大小写、scope 就是什么大小写——这里断言的正是这条不变量。
func TestOpenAIDedicatedQuotaPoolScope(t *testing.T) {
	t.Parallel()
	openai := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Equal(t, "GPT-5.3-Codex-Spark", openAIDedicatedQuotaPoolScope(openai, "GPT-5.3-Codex-Spark"))
	require.Equal(t, "gpt-5.3-codex-spark", openAIDedicatedQuotaPoolScope(openai, "gpt-5.3-codex-spark"))
	require.Empty(t, openAIDedicatedQuotaPoolScope(openai, "gpt-6-astra"))
	require.Empty(t, openAIDedicatedQuotaPoolScope(openai, ""))
	require.Empty(t, openAIDedicatedQuotaPoolScope(nil, "gpt-5.3-codex-spark"))

	grok := &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth}
	require.Empty(t, openAIDedicatedQuotaPoolScope(grok, "gpt-5.3-codex-spark"))
}

// 端到端（网关 fastpath）：spark 池的 429 不得触发内存里的整号熔断，
// 只应落一条模型级限流。2026-09-10 账号 8 在 spark 撞 429 后 7 秒内
// 连 gpt-6-astra 也被判限流，就是这条整号熔断放大出来的。
func TestOpenAI429FastPath_SparkPoolDoesNotRuntimeBlockWholeAccount(t *testing.T) {
	repo := &dedicatedQuotaPoolRepo{}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &OpenAIGatewayService{rateLimitService: rateLimitService}
	rateLimitService.SetAccountRuntimeBlocker(svc)

	account := &Account{ID: 8, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	shouldDisable := svc.handleOpenAIAccountUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		mainPoolHealthyCodexHeaders(),
		sparkPoolExhaustedBody(time.Now().Add(4*24*time.Hour)),
		"gpt-5.3-codex-spark",
	)

	require.False(t, shouldDisable)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account),
		"spark 池耗尽不得把整个账号运行时熔断")
	require.Zero(t, repo.setRateLimitedCalls)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.3-codex-spark", repo.modelRateLimitCalls[0].scope)
	require.False(t, svc.isOpenAIOAuth429Storm(), "spark 池的 429 不应计入全局 429 storm 计数")

	// 反向对照：主池模型的 429 仍然整号熔断 + 整号限流。
	mainPoolRepo := &dedicatedQuotaPoolRepo{}
	mainPoolRateLimitService := NewRateLimitService(mainPoolRepo, nil, &config.Config{}, nil, nil)
	mainPoolSvc := &OpenAIGatewayService{rateLimitService: mainPoolRateLimitService}
	mainPoolRateLimitService.SetAccountRuntimeBlocker(mainPoolSvc)
	mainPoolAccount := &Account{ID: 14, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	mainPoolSvc.handleOpenAIAccountUpstreamError(
		context.Background(),
		mainPoolAccount,
		http.StatusTooManyRequests,
		mainPoolHealthyCodexHeaders(),
		sparkPoolExhaustedBody(time.Now().Add(4*24*time.Hour)),
		"gpt-6-astra",
	)

	require.True(t, mainPoolSvc.isOpenAIAccountRuntimeBlocked(mainPoolAccount))
	require.Equal(t, 1, mainPoolRepo.setRateLimitedCalls)
	require.Empty(t, mainPoolRepo.modelRateLimitCalls)
}
