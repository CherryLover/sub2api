package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	modelRateLimitsKey                 = "model_rate_limits"
	antigravityGeminiModelRateLimitKey = "antigravity:gemini"
	openAIImageGenerationRateLimitKey  = "openai:image_generation"
	// anthropicFableRateLimitKey 是 Anthropic 7d_oi（Fable 专属 7d 窗口）限流的
	// 家族级 scope：命中后所有 Fable 变体（含 [1m] 等后缀）都不再调度到该账号。
	anthropicFableRateLimitKey = "claude-fable-5"
)

// isRateLimitActiveForKey 检查指定 key 的限流是否生效
func (a *Account) isRateLimitActiveForKey(key string) bool {
	resetAt := a.modelRateLimitResetAt(key)
	return resetAt != nil && time.Now().Before(*resetAt)
}

// getRateLimitRemainingForKey 获取指定 key 的限流剩余时间，0 表示未限流或已过期
func (a *Account) getRateLimitRemainingForKey(key string) time.Duration {
	resetAt := a.modelRateLimitResetAt(key)
	if resetAt == nil {
		return 0
	}
	remaining := time.Until(*resetAt)
	if remaining > 0 {
		return remaining
	}
	return 0
}

func (a *Account) isModelRateLimitedWithContext(ctx context.Context, requestedModel string) bool {
	for _, key := range a.modelRateLimitKeysForRequest(ctx, requestedModel) {
		if a.isRateLimitActiveForKey(key) {
			return true
		}
	}
	return false
}

// GetModelRateLimitRemainingTime 获取模型限流剩余时间
// 返回 0 表示未限流或已过期
func (a *Account) GetModelRateLimitRemainingTime(requestedModel string) time.Duration {
	return a.GetModelRateLimitRemainingTimeWithContext(context.Background(), requestedModel)
}

func (a *Account) GetModelRateLimitRemainingTimeWithContext(ctx context.Context, requestedModel string) time.Duration {
	remaining := time.Duration(0)
	for _, key := range a.modelRateLimitKeysForRequest(ctx, requestedModel) {
		if keyRemaining := a.getRateLimitRemainingForKey(key); keyRemaining > remaining {
			remaining = keyRemaining
		}
	}
	return remaining
}

func (a *Account) modelRateLimitKeysForRequest(ctx context.Context, requestedModel string) []string {
	if a == nil {
		return nil
	}

	modelKey := a.GetMappedModel(requestedModel)
	if a.Platform == PlatformAntigravity {
		modelKey = resolveFinalAntigravityModelKey(ctx, a, requestedModel)
	}
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return nil
	}

	keys := []string{modelKey}
	switch a.Platform {
	case PlatformAntigravity:
		if isAntigravityGeminiModel(modelKey) && modelKey != antigravityGeminiModelRateLimitKey {
			keys = append(keys, antigravityGeminiModelRateLimitKey)
		}
	case PlatformOpenAI:
		if openAIImageGenerationRateLimitApplies(ctx, requestedModel, modelKey) && modelKey != openAIImageGenerationRateLimitKey {
			keys = append(keys, openAIImageGenerationRateLimitKey)
		}
	case PlatformAnthropic:
		if isAnthropicFableModel(modelKey) && modelKey != anthropicFableRateLimitKey {
			keys = append(keys, anthropicFableRateLimitKey)
		}
	}
	return keys
}

// isAnthropicFableModel 判断是否为 Fable 模型家族（claude-fable-5、claude-fable-5[1m] 等变体）
func isAnthropicFableModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "fable")
}

// ── OpenAI 独立额度池模型 ──────────────────────────────────────────────────────
//
// 【上游的真实结构：额度池，不是模型】
// 查 /wham/usage 可以看到，一个 ChatGPT 账号的额度是按「池」（metered_feature）
// 组织的，不是按模型：
//   - 主池 `rate_limit`：gpt-5.5 / gpt-6-astra / gpt-5.6-* 等绝大多数模型共用；
//   - `additional_rate_limits` 里的附加池：目前上游只暴露一个 —— `codex_bengalfox`
//     （展示名 "GPT-5.3-Codex-Spark"）。
//
// 也就是说 gpt-5.3-codex-spark 有自己独立的一份周配额，它耗尽跟主池完全无关。
//
// 【为什么只能靠显式名单】
// 上游不提供「每模型用量」：model_usage 里只有 available / available_at，
// limit_name 也只是展示名，能查到的最小单位就是池。没有任何通用信号能从响应头或
// 响应体里推断"这个模型属于哪个池"，样本也不够去猜。所以模型 → 池的映射只能靠
// 这份显式名单维护；上游放出新的附加池时，往下面加一行即可（扩展点）。
//
// 【真实故障：spark 池满 → 整号限流】
// 2026-09-10 11:01:59 线上：账号 7 在 gpt-5.3-codex-spark 上撞 429，被按整号限流到
// 2026-09-15 14:37。事后核对 /wham/usage：该账号主池 7d 只用了 1%（重置时间是
// 09-18 09:32），spark 池 100%（重置时间正是 09-15 14:37）—— 限流截止时间与 spark
// 池的重置时间戳完全吻合，铁证是一个附加池的耗尽被当成了整号额度用尽。
// 后果：随后 5 分钟内所有模型共 230+ 次 503「无可用账号」（gpt-5.6-sol 150 次、
// gpt-5.5 30 次、gpt-6-astra 20 次、gpt-5.6-terra 10 次）；11:03:21 账号 8 在 spark
// 上撞 429，7 秒后它在完全无关的 gpt-6-astra 上也被判 429 限流。
//
// 因此名单内模型的 429 只能写「账号 × 模型」级限流（accounts.extra.model_rate_limits），
// 绝不能 fall through 到 SetRateLimited（整号，会写 accounts.rate_limit_reset_at）。
//
// 注意：gpt-5.6-sol 曾被怀疑有独立池，但 /wham/usage 显示上游只暴露了
// codex_bengalfox 一个附加池，sol 属于主池 —— 没有证据就不进名单，
// 误判的代价是这个模型的 429 不再冷却整号，请求会反复撞同一个已耗尽的账号。
var openAIDedicatedQuotaPoolModels = []string{
	// codex_bengalfox 池（展示名 GPT-5.3-Codex-Spark）
	"gpt-5.3-codex-spark",
}

// IsOpenAIDedicatedQuotaPoolModel 判断模型是否落在上游的独立额度池里。
// 入参可以是客户端请求名，也可以是账号映射后的上游名；大小写、首尾空白、
// 下划线、路径前缀（openai/xxx）与日期/后缀变体都会先归一化。
func IsOpenAIDedicatedQuotaPoolModel(model string) bool {
	return normalizeOpenAIDedicatedQuotaPoolModel(model) != ""
}

// normalizeOpenAIDedicatedQuotaPoolModel 归一化后返回命中的名单基名，未命中返回 ""。
func normalizeOpenAIDedicatedQuotaPoolModel(model string) string {
	normalized := canonicalizeOpenAIModelAliasSpelling(model)
	if normalized == "" {
		return ""
	}
	for _, name := range openAIDedicatedQuotaPoolModels {
		// 基名本身，或它的后缀变体（日期版本 gpt-5.3-codex-spark-2026-01-01、
		// gpt-5.3-codex-spark-openai-compact 等）都吃同一个池的配额。
		if normalized == name || strings.HasPrefix(normalized, name+"-") {
			return name
		}
	}
	return ""
}

// openAIDedicatedQuotaPoolScope 返回本次请求应写入 model_rate_limits 的 scope，
// 未命中独立额度池时返回 ""。
//
// 请求名与账号映射后的上游名都要判：
//   - 读取侧 modelRateLimitKeysForRequest 用的是 account.GetMappedModel(requestedModel)，
//     所以写入侧必须用同一个映射后的名字，否则写进去没人读；
//   - 请求名命中但映射后不是独立池模型（管理员把 spark 映射到了普通模型），说明真正
//     打到上游、真正被限流的不是独立池，这次 429 应该交回整号路径。
func openAIDedicatedQuotaPoolScope(account *Account, requestedModel string) string {
	model := strings.TrimSpace(requestedModel)
	if account == nil || model == "" || account.Platform != PlatformOpenAI {
		return ""
	}
	if !IsOpenAIDedicatedQuotaPoolModel(model) {
		return ""
	}
	scope := strings.TrimSpace(canonicalOpenAIAccountSchedulingModel(account, model))
	if scope == "" || !IsOpenAIDedicatedQuotaPoolModel(scope) {
		return ""
	}
	return scope
}

// isAccountModelRateLimitedForScheduling 是调度侧的模型级限流判定：账号对这个模型
// 是否正处于 accounts.extra.model_rate_limits 记录的限流窗口内。
//
// 单独抽出来是因为新版 OpenAI 调度器不走 IsSchedulableForModelWithContext
// （那条链只有 legacy 路径在用），却必须读到同一份数据 —— 否则模型级限流
// 写了没人读：账号照样被选中，请求继续撞同一个已耗尽的额度池。
// 语义与 IsSchedulableForModelWithContext 对齐，包括 Antigravity overages 例外。
func isAccountModelRateLimitedForScheduling(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil || strings.TrimSpace(requestedModel) == "" {
		return false
	}
	if !account.isModelRateLimitedWithContext(ctx, requestedModel) {
		return false
	}
	// Antigravity 开了 overages 且积分未耗尽时，模型级限流不拦截（有积分可用）。
	if account.Platform == PlatformAntigravity && account.IsOveragesEnabled() && !account.isCreditsExhausted() {
		return false
	}
	return true
}

func openAIImageGenerationRateLimitApplies(ctx context.Context, requestedModel, modelKey string) bool {
	if isOpenAIImageGenerationModel(requestedModel) || isOpenAIImageGenerationModel(modelKey) {
		return true
	}
	return OpenAIImageGenerationIntentFromContext(ctx)
}

func WithOpenAIImageGenerationIntent(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxkey.OpenAIImageGenerationIntent, true)
}

func OpenAIImageGenerationIntentFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, ok := ctx.Value(ctxkey.OpenAIImageGenerationIntent).(bool)
	return ok && enabled
}

// WithOpenAIImagesEndpoint 标记请求从 /v1/images/* 专用生图端点入站。
func WithOpenAIImagesEndpoint(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxkey.OpenAIImagesEndpoint, true)
}

// OpenAIImagesEndpointFromContext 报告请求是否来自 /v1/images/*。
func OpenAIImagesEndpointFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, ok := ctx.Value(ctxkey.OpenAIImagesEndpoint).(bool)
	return ok && enabled
}

func resolveFinalAntigravityModelKey(ctx context.Context, account *Account, requestedModel string) string {
	modelKey := mapAntigravityModel(account, requestedModel)
	if modelKey == "" {
		return ""
	}
	// thinking 会影响 Antigravity 最终模型名（例如 claude-sonnet-4-5 -> claude-sonnet-4-5-thinking）
	if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
		modelKey = applyThinkingModelSuffix(modelKey, enabled)
	}
	return modelKey
}

func isAntigravityGeminiModel(model string) bool {
	return strings.HasPrefix(normalizeAntigravityModelName(model), "gemini-")
}

func antigravityModelRateLimitKeys(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	keys := []string{model}
	if isAntigravityGeminiModel(model) && model != antigravityGeminiModelRateLimitKey {
		keys = append(keys, antigravityGeminiModelRateLimitKey)
	}
	return keys
}

func (a *Account) modelRateLimitResetAt(scope string) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
	}
	rawLimits, ok := a.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return nil
	}
	rawLimit, ok := rawLimits[scope].(map[string]any)
	if !ok {
		return nil
	}
	resetAtRaw, ok := rawLimit["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
	}
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
	}
	return &resetAt
}

func setAccountModelRateLimitSnapshot(account *Account, scope string, resetAt time.Time, reason string, now time.Time) {
	if account == nil || strings.TrimSpace(scope) == "" {
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	limits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		limits = make(map[string]any)
		account.Extra[modelRateLimitsKey] = limits
	}
	payload := map[string]any{
		"rate_limited_at":     now.UTC().Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		payload["reason"] = reason
	}
	limits[scope] = payload
}
