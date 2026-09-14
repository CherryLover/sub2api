//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// openAIWhamUsageSample 是实测的 /wham/usage 顶层结构（12 个键，数值做过脱敏）。
// 历史上我们只解析了其中 5 个，model_usage / code_review_rate_limit / spend_control /
// credits / rate_limit_reached_type 全被静默丢弃 —— 这份样本就是回归基线。
const openAIWhamUsageSample = `{
  "user_id": "user-abc",
  "account_id": "acct-abc",
  "email": "someone@example.com",
  "plan_type": "pro",
  "rate_limit": {
    "allowed": true,
    "limit_reached": false,
    "primary_window": {"used_percent": 12.5, "limit_window_seconds": 18000, "reset_after_seconds": 3600, "reset_at": 1789370776},
    "secondary_window": {"used_percent": 30.0, "limit_window_seconds": 604800, "reset_after_seconds": 86400, "reset_at": 1789470776}
  },
  "code_review_rate_limit": null,
  "additional_rate_limits": [
    {
      "limit_name": "GPT-5.3-Codex-Spark",
      "metered_feature": "codex_bengalfox",
      "rate_limit": {
        "allowed": true,
        "limit_reached": false,
        "primary_window": {"used_percent": 0, "limit_window_seconds": 18000, "reset_after_seconds": 18000, "reset_at": 1789370776},
        "secondary_window": {"used_percent": 0, "limit_window_seconds": 604800, "reset_after_seconds": 604800, "reset_at": 1789970776}
      },
      "normal_model_slug": null
    }
  ],
  "model_usage": {
    "gpt-6-astra": {"available": true, "available_at": null, "credits_would_enable": false}
  },
  "credits": {"has_credits": false, "unlimited": false, "overage_limit_reached": false, "balance": "0"},
  "spend_control": {"reached": false, "individual_limit": null},
  "rate_limit_reached_type": null,
  "promo": null,
  "rate_limit_reset_credits": {"available_count": 2}
}`

// TestOpenAIQuotaUsage_ParsesPreviouslyDroppedTopLevelFields 锁住任务一：12 个顶层键里
// 之前漏读的那几个必须能解析出来，而且 null 要保持"没信息"的语义（不能塌成零值真值）。
func TestOpenAIQuotaUsage_ParsesPreviouslyDroppedTopLevelFields(t *testing.T) {
	t.Parallel()

	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal([]byte(openAIWhamUsageSample), &usage))

	// 既有字段的语义一个都不能变（管理后台的额度卡片依赖它们）。
	require.Equal(t, "pro", usage.PlanType)
	require.NotNil(t, usage.RateLimit)
	require.True(t, usage.RateLimit.Allowed)
	require.NotNil(t, usage.RateLimitResetCredits)
	require.Equal(t, 2, usage.RateLimitResetCredits.AvailableCount)

	// code_review_rate_limit：实测 null ⇒ 指针为 nil（"没有这个桶"，不是"空桶"）。
	require.Nil(t, usage.CodeReviewRateLimit)

	// rate_limit_reached_type：实测 null ⇒ 空串（当前没触顶）。
	require.Empty(t, usage.RateLimitReachedType)

	// additional_rate_limits.normal_model_slug 之前也没读。
	require.Len(t, usage.AdditionalRateLimits, 1)
	require.Equal(t, "codex_bengalfox", usage.AdditionalRateLimits[0].MeteredFeature)
	require.Equal(t, "GPT-5.3-Codex-Spark", usage.AdditionalRateLimits[0].LimitName)
	require.Empty(t, usage.AdditionalRateLimits[0].NormalModelSlug)
	require.NotNil(t, usage.AdditionalRateLimits[0].RateLimit)
	require.NotNil(t, usage.AdditionalRateLimits[0].RateLimit.SecondaryWindow)
	require.Equal(t, int64(1789970776), usage.AdditionalRateLimits[0].RateLimit.SecondaryWindow.ResetAt)

	// model_usage：显式 true 必须能与"没提" 区分开，所以是 *bool。
	require.Len(t, usage.ModelUsage, 1)
	astra, ok := usage.ModelUsage["gpt-6-astra"]
	require.True(t, ok)
	require.NotNil(t, astra.Available)
	require.True(t, *astra.Available)
	require.NotNil(t, astra.CreditsWouldEnable)
	require.False(t, *astra.CreditsWouldEnable)
	_, hasAvailableAt := astra.AvailableAtTime()
	require.False(t, hasAvailableAt, "available_at 为 null 时必须报告为「没信息」")

	// credits / spend_control：类型未知的标量原样透传，不猜类型。
	require.NotNil(t, usage.Credits)
	require.False(t, usage.Credits.HasCredits)
	require.False(t, usage.Credits.Unlimited)
	require.False(t, usage.Credits.OverageLimitReached)
	require.JSONEq(t, `"0"`, string(usage.Credits.Balance))
	require.NotNil(t, usage.SpendControl)
	require.False(t, usage.SpendControl.Reached)
	require.JSONEq(t, `null`, string(usage.SpendControl.IndividualLimit))
}

// TestOpenAIQuotaUsage_MissingNewFieldsStayZero 覆盖"整块缺失"（老账号 / 上游裁剪响应）：
// 缺失与 null 都必须落到"没信息"，绝不能变成"可用"或"不可用"的断言。
func TestOpenAIQuotaUsage_MissingNewFieldsStayZero(t *testing.T) {
	t.Parallel()

	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"plan_type":"plus","rate_limit":{"allowed":true}}`), &usage))

	require.Equal(t, "plus", usage.PlanType)
	require.Nil(t, usage.CodeReviewRateLimit)
	require.Nil(t, usage.ModelUsage)
	require.Nil(t, usage.Credits)
	require.Nil(t, usage.SpendControl)
	require.Empty(t, usage.RateLimitReachedType)

	// 序列化回去时新字段必须全部消失，否则管理后台的既有响应形状被改了。
	encoded, err := json.Marshal(&usage)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "model_usage")
	require.NotContains(t, string(encoded), "code_review_rate_limit")
	require.NotContains(t, string(encoded), "spend_control")
	require.NotContains(t, string(encoded), "credits")
}

// TestOpenAIModelUsage_AvailableAtTime 覆盖 available_at 的各种写法。上游没有文档，
// 所以解析必须兼容 unix 秒 / unix 毫秒 / RFC3339 字符串，并且对不认识的写法报"没信息"。
func TestOpenAIModelUsage_AvailableAtTime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want time.Time
		ok   bool
	}{
		{name: "missing", raw: "", ok: false},
		{name: "null", raw: "null", ok: false},
		{name: "unix seconds", raw: "1789370776", want: time.Unix(1789370776, 0).UTC(), ok: true},
		{name: "unix millis", raw: "1789370776000", want: time.Unix(1789370776, 0).UTC(), ok: true},
		{name: "rfc3339 string", raw: `"2026-09-15T14:37:00Z"`, want: time.Date(2026, 9, 15, 14, 37, 0, 0, time.UTC), ok: true},
		{name: "numeric string", raw: `"1789370776"`, want: time.Unix(1789370776, 0).UTC(), ok: true},
		{name: "garbage string", raw: `"soon"`, ok: false},
		{name: "object", raw: `{"at":1}`, ok: false},
		{name: "zero", raw: "0", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usage := OpenAIModelUsage{}
			if tc.raw != "" {
				usage.AvailableAt = json.RawMessage(tc.raw)
			}
			got, ok := usage.AvailableAtTime()
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.True(t, tc.want.Equal(got), "want %s got %s", tc.want, got)
			}
		})
	}
}

// ── 判定层（planOpenAIModelRateLimitSelfHeal）──────────────────────────────────

func openAIModelSelfHealBoolPtr(v bool) *bool { return &v }

func openAIModelSelfHealSparkPool(allowed, limitReached bool, primaryPercent, secondaryPercent float64) []OpenAIAdditionalRateLimit {
	return []OpenAIAdditionalRateLimit{
		{
			LimitName:      "GPT-5.3-Codex-Spark",
			MeteredFeature: openAICodexSparkMeteredFeature,
			RateLimit: &OpenAIRateLimit{
				Allowed:         allowed,
				LimitReached:    limitReached,
				PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: primaryPercent, LimitWindowSeconds: 18000},
				SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: secondaryPercent, LimitWindowSeconds: 604800},
			},
		},
	}
}

// TestPlanSelfHeal_DedicatedPoolHealthyClearsSpark 复刻 2026-09-11 那次事故的恢复时刻：
// 我们锁到 09-15，而上游 09-14 就把 codex_bengalfox 滚动重置了（allowed=true、
// limit_reached=false、两个窗口 0%）。这时必须解除，而且只解除 spark 那一条。
func TestPlanSelfHeal_DedicatedPoolHealthyClearsSpark(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	sparkResetAt := now.Add(30 * time.Hour).Truncate(time.Second)
	observed := map[string]time.Time{
		"gpt-5.3-codex-spark": sparkResetAt,
		"gpt-5.6-sol":         now.Add(2 * time.Hour).Truncate(time.Second),
	}
	usage := &OpenAIQuotaUsage{
		RateLimit:            &OpenAIRateLimit{Allowed: true},
		AdditionalRateLimits: openAIModelSelfHealSparkPool(true, false, 0, 0),
	}

	clears, blocks := planOpenAIModelRateLimitSelfHeal(observed, usage, now)

	require.Empty(t, blocks)
	require.Len(t, clears, 1, "上游只为 spark 池给了正面证据，别的模型不能顺带解除")
	require.Equal(t, "gpt-5.3-codex-spark", clears[0].scope)
	require.True(t, sparkResetAt.Equal(clears[0].observedResetAt),
		"条件清除必须带上观测到的那一代 reset_at")
	require.Contains(t, clears[0].evidence, openAICodexSparkMeteredFeature)
}

// TestPlanSelfHeal_DedicatedPoolStillExhaustedKeepsLimit 池仍然满 / 上游仍说不允许时，
// 一律不解除。这是保守原则的底线：没有正面证据就继续锁着。
func TestPlanSelfHeal_DedicatedPoolStillExhaustedKeepsLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	observed := map[string]time.Time{"gpt-5.3-codex-spark": now.Add(30 * time.Hour)}

	cases := []struct {
		name  string
		usage *OpenAIQuotaUsage
	}{
		{
			name:  "limit_reached",
			usage: &OpenAIQuotaUsage{AdditionalRateLimits: openAIModelSelfHealSparkPool(false, true, 100, 100)},
		},
		{
			name:  "secondary window full",
			usage: &OpenAIQuotaUsage{AdditionalRateLimits: openAIModelSelfHealSparkPool(true, false, 0, 100)},
		},
		{
			name:  "primary window full",
			usage: &OpenAIQuotaUsage{AdditionalRateLimits: openAIModelSelfHealSparkPool(true, false, 100, 0)},
		},
		{
			name: "window missing",
			usage: &OpenAIQuotaUsage{AdditionalRateLimits: []OpenAIAdditionalRateLimit{{
				MeteredFeature: openAICodexSparkMeteredFeature,
				RateLimit:      &OpenAIRateLimit{Allowed: true},
			}}},
		},
		{
			name:  "pool entry absent",
			usage: &OpenAIQuotaUsage{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clears, blocks := planOpenAIModelRateLimitSelfHeal(observed, tc.usage, now)
			require.Empty(t, clears)
			require.Empty(t, blocks)
		})
	}
}

// TestPlanSelfHeal_MainPoolHealthDoesNotClearSpark 钉死"不同池不能互相推翻"：
// 主池（rate_limit）完全健康、model_usage 甚至没提 spark 时，spark 的限流必须原样保留。
// 反向（2026-09-10）那次事故已经证明了把两个池混为一谈的代价。
func TestPlanSelfHeal_MainPoolHealthDoesNotClearSpark(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	observed := map[string]time.Time{"gpt-5.3-codex-spark": now.Add(30 * time.Hour)}
	usage := &OpenAIQuotaUsage{
		RateLimit: &OpenAIRateLimit{
			Allowed:         true,
			LimitReached:    false,
			PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 1},
			SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 1},
		},
		// 主池健康，但这次响应里根本没有 codex_bengalfox 的条目。
		AdditionalRateLimits: []OpenAIAdditionalRateLimit{{
			MeteredFeature: "some_other_pool",
			RateLimit:      &OpenAIRateLimit{Allowed: true, PrimaryWindow: &OpenAIRateLimitWindow{}, SecondaryWindow: &OpenAIRateLimitWindow{}},
		}},
	}

	clears, blocks := planOpenAIModelRateLimitSelfHeal(observed, usage, now)
	require.Empty(t, clears, "主池健康不能用来解除 spark 的限流")
	require.Empty(t, blocks)
}

// TestPlanSelfHeal_ExhaustedPoolBeatsModelUsageAvailable 池条目在场且不健康时，
// model_usage 的 available=true 也不能推翻它（池是这个模型的权威数据源）。
func TestPlanSelfHeal_ExhaustedPoolBeatsModelUsageAvailable(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	observed := map[string]time.Time{"gpt-5.3-codex-spark": now.Add(30 * time.Hour)}
	usage := &OpenAIQuotaUsage{
		AdditionalRateLimits: openAIModelSelfHealSparkPool(true, false, 100, 0),
		ModelUsage: map[string]OpenAIModelUsage{
			"gpt-5.3-codex-spark": {Available: openAIModelSelfHealBoolPtr(true)},
		},
	}

	clears, _ := planOpenAIModelRateLimitSelfHeal(observed, usage, now)
	require.Empty(t, clears)
}

// TestPlanSelfHeal_SparseModelUsageLeavesUnmentionedModelsAlone 覆盖稀疏语义：
// model_usage 只提了一个模型，其它模型的限流必须一动不动 —— "没被列出来" ≠ 可用，
// 也 ≠ 不可用。同时命名空间 key（openai:image_generation）永远不参与。
func TestPlanSelfHeal_SparseModelUsageLeavesUnmentionedModelsAlone(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	observed := map[string]time.Time{
		"gpt-6-astra":                     now.Add(2 * time.Hour).Truncate(time.Second),
		"gpt-5.6-sol":                     now.Add(3 * time.Hour).Truncate(time.Second),
		"gpt-5.6-terra":                   now.Add(4 * time.Hour).Truncate(time.Second),
		openAIImageGenerationRateLimitKey: now.Add(5 * time.Hour).Truncate(time.Second),
	}
	usage := &OpenAIQuotaUsage{
		ModelUsage: map[string]OpenAIModelUsage{
			// 只有 astra 被显式放行；terra 被提到了但没给 available（依然是"没信息"）。
			"gpt-6-astra":   {Available: openAIModelSelfHealBoolPtr(true)},
			"gpt-5.6-terra": {CreditsWouldEnable: openAIModelSelfHealBoolPtr(true)},
		},
	}

	clears, blocks := planOpenAIModelRateLimitSelfHeal(observed, usage, now)
	require.Empty(t, blocks)
	require.Len(t, clears, 1)
	require.Equal(t, "gpt-6-astra", clears[0].scope)
	require.Equal(t, "model_usage_available", clears[0].evidence)
}

// TestPlanSelfHeal_ModelUsageUnavailableWritesLimit 覆盖反向：上游显式说不可用时
// 要据此写入限流，available_at 有值就用它当解除时间。
func TestPlanSelfHeal_ModelUsageUnavailableWritesLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	availableAt := now.Add(90 * time.Minute).Truncate(time.Second)
	usage := &OpenAIQuotaUsage{
		ModelUsage: map[string]OpenAIModelUsage{
			"gpt-6-astra": {
				Available:   openAIModelSelfHealBoolPtr(false),
				AvailableAt: json.RawMessage(fmt.Sprintf("%d", availableAt.Unix())),
			},
			"gpt-5.6-sol": {Available: openAIModelSelfHealBoolPtr(false)},
		},
	}

	clears, blocks := planOpenAIModelRateLimitSelfHeal(nil, usage, now)
	require.Empty(t, clears)
	require.Len(t, blocks, 2)

	byScope := make(map[string]openAIModelRateLimitBlock, len(blocks))
	for _, block := range blocks {
		byScope[block.scope] = block
	}

	astra, ok := byScope["gpt-6-astra"]
	require.True(t, ok)
	require.True(t, availableAt.Equal(astra.resetAt), "有 available_at 就必须用它当解除时间")
	require.Equal(t, "model_usage_unavailable_with_available_at", astra.evidence)

	sol, ok := byScope["gpt-5.6-sol"]
	require.True(t, ok)
	require.True(t, sol.resetAt.Equal(now.Add(openAIModelUsageFallbackCooldown)),
		"没有 available_at 时只敢冷却一小段，等下次刷新重新判定")
	require.Equal(t, "model_usage_unavailable", sol.evidence)
}

// TestPlanSelfHeal_ModelUsageUnavailableNeverShortensExistingLimit 上游说不可用、但
// 库里已经有一条更长的限流（通常来自一次真实 429）时，绝不能把它改短。
func TestPlanSelfHeal_ModelUsageUnavailableNeverShortensExistingLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	observed := map[string]time.Time{"gpt-6-astra": now.Add(6 * time.Hour).Truncate(time.Second)}
	usage := &OpenAIQuotaUsage{
		ModelUsage: map[string]OpenAIModelUsage{
			"gpt-6-astra": {Available: openAIModelSelfHealBoolPtr(false)},
		},
	}

	clears, blocks := planOpenAIModelRateLimitSelfHeal(observed, usage, now)
	require.Empty(t, clears, "显式不可用时绝不解除")
	require.Empty(t, blocks, "已有的限流更长，不允许被缩短")
}

// TestPlanSelfHeal_ModelUsageAvailableAtIsClamped 脏的 available_at（比如 2099 年）
// 必须被 7d 上限截断 —— 本文件开头的事故正是"被锁在一个过长的未来时间点"。
func TestPlanSelfHeal_ModelUsageAvailableAtIsClamped(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	usage := &OpenAIQuotaUsage{
		ModelUsage: map[string]OpenAIModelUsage{
			"gpt-6-astra": {
				Available:   openAIModelSelfHealBoolPtr(false),
				AvailableAt: json.RawMessage(`"2099-01-01T00:00:00Z"`),
			},
		},
	}

	_, blocks := planOpenAIModelRateLimitSelfHeal(nil, usage, now)
	require.Len(t, blocks, 1)
	require.True(t, blocks[0].resetAt.Equal(now.Add(openAIModelUsageMaxCooldown)))
	require.Contains(t, blocks[0].evidence, "clamped")
}

// TestPlanSelfHeal_NoUsageOrNoObservationIsNoop 空输入必须是纯 no-op。
func TestPlanSelfHeal_NoUsageOrNoObservationIsNoop(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	clears, blocks := planOpenAIModelRateLimitSelfHeal(map[string]time.Time{"gpt-6-astra": now.Add(time.Hour)}, nil, now)
	require.Empty(t, clears)
	require.Empty(t, blocks)

	clears, blocks = planOpenAIModelRateLimitSelfHeal(nil, &OpenAIQuotaUsage{}, now)
	require.Empty(t, clears)
	require.Empty(t, blocks)
}

// ── 落库层（QueryUsage → applyOpenAIModelUsageSelfHeal）─────────────────────────

type openAIModelSelfHealClearCall struct {
	scope           string
	observedResetAt time.Time
}

type openAIModelSelfHealSetCall struct {
	scope   string
	resetAt time.Time
	reason  string
}

// openAIModelSelfHealRepo 模拟 accounts 行：ClearModelRateLimitIfObserved 的语义与真实
// SQL 一致（reset_at 字符串必须与观测值完全相同才删），因此能真实地重现"往返期间代际
// 变化"这个丢失更新场景。任何账号级写操作都会被记下来供断言。
type openAIModelSelfHealRepo struct {
	AccountRepository

	mu                sync.Mutex
	account           *Account
	clearCalls        []openAIModelSelfHealClearCall
	setCalls          []openAIModelSelfHealSetCall
	accountLevelCalls []string
}

func (r *openAIModelSelfHealRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account == nil || r.account.ID != id {
		return nil, fmt.Errorf("account %d not found", id)
	}
	return r.account, nil
}

func (r *openAIModelSelfHealRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	return nil
}

func (r *openAIModelSelfHealRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, reason ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := openAIModelSelfHealSetCall{scope: scope, resetAt: resetAt}
	if len(reason) > 0 {
		call.reason = reason[0]
	}
	r.setCalls = append(r.setCalls, call)
	r.writeLimitLocked(scope, resetAt)
	return nil
}

func (r *openAIModelSelfHealRepo) ClearModelRateLimitIfObserved(_ context.Context, _ int64, scope string, observedResetAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearCalls = append(r.clearCalls, openAIModelSelfHealClearCall{scope: scope, observedResetAt: observedResetAt})
	limits, ok := r.account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return false, nil
	}
	entry, ok := limits[scope].(map[string]any)
	if !ok {
		return false, nil
	}
	current, _ := entry["rate_limit_reset_at"].(string)
	if current == "" || current != observedResetAt.UTC().Format(time.RFC3339) {
		return false, nil
	}
	delete(limits, scope)
	return true, nil
}

// 账号级写入口：自愈绝不允许碰它们，调用即记账，测试断言必须为空。
func (r *openAIModelSelfHealRepo) SetRateLimited(_ context.Context, _ int64, _ time.Time) error {
	r.recordAccountLevelCall("SetRateLimited")
	return nil
}

func (r *openAIModelSelfHealRepo) ClearRateLimit(_ context.Context, _ int64) error {
	r.recordAccountLevelCall("ClearRateLimit")
	return nil
}

func (r *openAIModelSelfHealRepo) ClearOpenAIRateLimitIfObserved(_ context.Context, _ int64, _, _ time.Time) (bool, error) {
	r.recordAccountLevelCall("ClearOpenAIRateLimitIfObserved")
	return false, nil
}

func (r *openAIModelSelfHealRepo) SetOverloaded(_ context.Context, _ int64, _ time.Time) error {
	r.recordAccountLevelCall("SetOverloaded")
	return nil
}

func (r *openAIModelSelfHealRepo) ClearModelRateLimits(_ context.Context, _ int64) error {
	r.recordAccountLevelCall("ClearModelRateLimits")
	return nil
}

func (r *openAIModelSelfHealRepo) recordAccountLevelCall(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accountLevelCalls = append(r.accountLevelCalls, name)
}

func (r *openAIModelSelfHealRepo) writeLimitLocked(scope string, resetAt time.Time) {
	limits, ok := r.account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		limits = make(map[string]any)
		r.account.Extra[modelRateLimitsKey] = limits
	}
	limits[scope] = map[string]any{
		"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}
}

func (r *openAIModelSelfHealRepo) modelRateLimitResetAt(scope string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	limits, ok := r.account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return ""
	}
	entry, ok := limits[scope].(map[string]any)
	if !ok {
		return ""
	}
	resetAt, _ := entry["rate_limit_reset_at"].(string)
	return resetAt
}

// newOpenAIModelSelfHealAccount 造一个普通（非影子）OpenAI OAuth 账号，带一条模型级限流。
func newOpenAIModelSelfHealAccount(scope string, resetAt time.Time) *Account {
	return &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-self-heal",
		},
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				scope: map[string]any{
					"rate_limited_at":     time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
					"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
					"reason":              "openai_dedicated_quota_pool_rate_limited",
				},
			},
		},
	}
}

// newOpenAIModelSelfHealService 把 quota service 接到 httptest 上；onUsage 在服务端返回
// 之前执行，用来模拟"往返期间库里发生了变化"。
func newOpenAIModelSelfHealService(
	t *testing.T,
	repo *openAIModelSelfHealRepo,
	payload func() OpenAIQuotaUsage,
	onUsage func(),
) *OpenAIQuotaService {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			// reset-credit 明细是可选增强，失败只会打日志，不影响自愈。
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if onUsage != nil {
			onUsage()
		}
		w.Header().Set("content-type", "application/json")
		resp := payload()
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(repo.account): "fake-access-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)
	return NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
}

// TestQueryUsage_SelfHealClearsSparkWhenPoolIsHealthy 端到端复刻事故的恢复：一次普通的
// 用量查询就应该把那条"锁到明天"的 spark 限流解除掉，且只用条件清除。
func TestQueryUsage_SelfHealClearsSparkWhenPoolIsHealthy(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(30 * time.Hour).UTC().Truncate(time.Second)
	repo := &openAIModelSelfHealRepo{account: newOpenAIModelSelfHealAccount("gpt-5.3-codex-spark", resetAt)}
	svc := newOpenAIModelSelfHealService(t, repo, func() OpenAIQuotaUsage {
		return OpenAIQuotaUsage{AdditionalRateLimits: openAIModelSelfHealSparkPool(true, false, 0, 0)}
	}, nil)

	usage, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)

	require.Len(t, repo.clearCalls, 1)
	require.Equal(t, "gpt-5.3-codex-spark", repo.clearCalls[0].scope)
	require.True(t, resetAt.Equal(repo.clearCalls[0].observedResetAt))
	require.Empty(t, repo.setCalls)
	require.Empty(t, repo.modelRateLimitResetAt("gpt-5.3-codex-spark"), "限流条目应已被移除")

	// 只动模型级：账号级的任何写入口都不许被调用，账号级字段保持原样。
	require.Empty(t, repo.accountLevelCalls)
	require.Nil(t, repo.account.RateLimitedAt)
	require.Nil(t, repo.account.RateLimitResetAt)
	require.Nil(t, repo.account.OverloadUntil)
}

// TestQueryUsage_SelfHealSkipsWhenGenerationChanged 是丢失更新防护：/wham/usage 往返
// 期间一个真实 429 写入了更长的限流，这时条件清除必须落空，新的限流原样保留。
func TestQueryUsage_SelfHealSkipsWhenGenerationChanged(t *testing.T) {
	t.Parallel()

	observedResetAt := time.Now().Add(30 * time.Hour).UTC().Truncate(time.Second)
	newerResetAt := time.Now().Add(60 * time.Hour).UTC().Truncate(time.Second)
	repo := &openAIModelSelfHealRepo{account: newOpenAIModelSelfHealAccount("gpt-5.3-codex-spark", observedResetAt)}

	svc := newOpenAIModelSelfHealService(t, repo, func() OpenAIQuotaUsage {
		return OpenAIQuotaUsage{AdditionalRateLimits: openAIModelSelfHealSparkPool(true, false, 0, 0)}
	}, func() {
		// 模拟往返期间的真实 429：SetModelRateLimit 写入新的一代。
		_ = repo.SetModelRateLimit(context.Background(), repo.account.ID, "gpt-5.3-codex-spark", newerResetAt)
	})

	_, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)

	require.Len(t, repo.clearCalls, 1, "仍然要尝试，但必须带着观测代际去匹配")
	require.True(t, observedResetAt.Equal(repo.clearCalls[0].observedResetAt))
	require.Equal(t, newerResetAt.Format(time.RFC3339), repo.modelRateLimitResetAt("gpt-5.3-codex-spark"),
		"代际变了就什么都不做，往返期间写入的限流必须原样保留")
	require.Empty(t, repo.accountLevelCalls)
}

// TestQueryUsage_SelfHealWritesLimitFromModelUsageUnavailable 覆盖反向落库：上游显式说
// 某个模型不可用时，要写入模型级限流并采用 available_at。
func TestQueryUsage_SelfHealWritesLimitFromModelUsageUnavailable(t *testing.T) {
	t.Parallel()

	availableAt := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	repo := &openAIModelSelfHealRepo{account: newOpenAIModelSelfHealAccount("gpt-5.3-codex-spark", time.Now().Add(30*time.Hour).UTC().Truncate(time.Second))}
	svc := newOpenAIModelSelfHealService(t, repo, func() OpenAIQuotaUsage {
		return OpenAIQuotaUsage{
			// spark 池仍然满：那一条限流不能被解除。
			AdditionalRateLimits: openAIModelSelfHealSparkPool(false, true, 100, 100),
			ModelUsage: map[string]OpenAIModelUsage{
				"gpt-6-astra": {
					Available:   openAIModelSelfHealBoolPtr(false),
					AvailableAt: json.RawMessage(fmt.Sprintf("%d", availableAt.Unix())),
				},
			},
		}
	}, nil)

	_, err := svc.QueryUsage(context.Background(), repo.account.ID)
	require.NoError(t, err)

	require.Empty(t, repo.clearCalls)
	require.Len(t, repo.setCalls, 1)
	require.Equal(t, "gpt-6-astra", repo.setCalls[0].scope)
	require.True(t, availableAt.Equal(repo.setCalls[0].resetAt))
	require.Equal(t, openAIModelUsageUnavailableReason, repo.setCalls[0].reason)
	require.Empty(t, repo.accountLevelCalls)
}

// TestObserveModelRateLimitsForSelfHeal_SkipsShadowAccounts spark 影子不参与模型级自愈：
// 它整行只承接 bengalfox 一道额度，而 /wham/usage 返回的其实是母账号的表态。
func TestObserveModelRateLimitsForSelfHeal_SkipsShadowAccounts(t *testing.T) {
	t.Parallel()

	parentID := int64(6)
	shadow := newOpenAIModelSelfHealAccount("gpt-5.3-codex-spark", time.Now().Add(30*time.Hour))
	shadow.ParentAccountID = &parentID
	shadow.QuotaDimension = QuotaDimensionSpark
	repo := &openAIModelSelfHealRepo{account: shadow}
	svc := NewOpenAIQuotaService(repo, nil, nil, nil)

	require.Nil(t, svc.observeModelRateLimitsForSelfHeal(context.Background(), shadow.ID, time.Now()))

	// 非 OpenAI 平台同样不参与。
	repo.account.ParentAccountID = nil
	repo.account.QuotaDimension = ""
	repo.account.Platform = PlatformAnthropic
	require.Nil(t, svc.observeModelRateLimitsForSelfHeal(context.Background(), shadow.ID, time.Now()))
}

// TestActiveModelRateLimits_OnlyReturnsLiveEntries 观测快照只收还在生效的条目，
// 并且原样带回解除时间（它就是条件清除要匹配的那一代）。
func TestActiveModelRateLimits_OnlyReturnsLiveEntries(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	live := now.Add(2 * time.Hour).Truncate(time.Second)
	account := &Account{
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"gpt-6-astra":   map[string]any{"rate_limit_reset_at": live.Format(time.RFC3339)},
				"gpt-5.6-sol":   map[string]any{"rate_limit_reset_at": now.Add(-time.Hour).Format(time.RFC3339)},
				"gpt-5.6-terra": map[string]any{"reason": "no reset at"},
			},
		},
	}

	limits := account.activeModelRateLimits(now)
	require.Len(t, limits, 1)
	require.True(t, live.Equal(limits["gpt-6-astra"]))

	require.Nil(t, (&Account{}).activeModelRateLimits(now))
	require.Nil(t, (*Account)(nil).activeModelRateLimits(now))
}

// TestOpenAIDedicatedQuotaPoolMeteredFeatureFor 模型 → 池的映射要能吃下拼写变体，
// 未命中时必须返回空（没有池就没有解除证据）。
func TestOpenAIDedicatedQuotaPoolMeteredFeatureFor(t *testing.T) {
	t.Parallel()

	require.Equal(t, openAICodexSparkMeteredFeature, openAIDedicatedQuotaPoolMeteredFeatureFor("gpt-5.3-codex-spark"))
	require.Equal(t, openAICodexSparkMeteredFeature, openAIDedicatedQuotaPoolMeteredFeatureFor("GPT-5.3-Codex-Spark"))
	require.Equal(t, openAICodexSparkMeteredFeature, openAIDedicatedQuotaPoolMeteredFeatureFor("gpt-5.3-codex-spark-2026-01-01"))
	require.Empty(t, openAIDedicatedQuotaPoolMeteredFeatureFor("gpt-6-astra"))
	require.Empty(t, openAIDedicatedQuotaPoolMeteredFeatureFor(""))
}
