package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// ── 模型级限流的自愈 ──────────────────────────────────────────────────────────
//
// 【真实事故：上游早就放开了，我们还锁到明天】
// 2026-09-11，某账号在 gpt-5.3-codex-spark 上撞 429，上游当时说要到 09-15 才恢复，
// 我们照着写进了 accounts.extra.model_rate_limits。但 spark 的 7d 窗口是滚动的：
// 09-14 10:17 那个池子就自己提前重置了 —— /wham/usage 里 codex_bengalfox 明明白白写着
// allowed=true、limit_reached=false、两个窗口 used_percent 都是 0，而我们仍然锁到
// 09-15/09-16，客户端在这段时间里一直收到**我们自己发的** 503「无可用账号」。
//
// 根因不是判定写错了，而是模型级限流**根本没有任何自愈入口**：写进去之后只能干等
// rate_limit_reset_at 到点。账号级限流早在 clearOpenAIRateLimitIfCodexSnapshotHealthy
// （account_usage_service.go）就有自愈了，模型级一直是个缺口。这个文件补上它。
//
// 【为什么数据源只能是 /wham/usage】
// /responses 的 x-codex-* 响应头不带池名，无法自证属于哪个额度池；只有 /wham/usage 能
// 按池读到真实余量（additional_rate_limits），也只有它带 model_usage —— 上游对"这个
// 模型现在能不能用"的**唯一显式表态**。
//
// 【第一原则：保守】
// 只在拿到**正面证据**时解除，绝不因为"没有信息"就解除：
//
//   - 独立额度池模型（目前只有 spark / codex_bengalfox）：必须它自己那个池
//     allowed=true、limit_reached=false，且两个窗口 used_percent 都 < 100；
//   - model_usage 里显式 available=true：解除；
//   - model_usage 里显式 available=false：不但不解除，反而据此写入/延长限流
//     （有 available_at 就用它当解除时间）—— 这比靠 429 反推准确得多；
//   - 上游没提到的模型：什么都不做，保持现状。
//
// model_usage 是**稀疏**的：没被列出来 ≠ 不可用，也 ≠ 可用，就是"没信息"。所以判定
// 一律基于 *bool 的三态，绝不把缺失当 false（见 OpenAIModelUsage 的注释）。
//
// 【必须守住的边界】（照抄账号级自愈的教训）
//
//  1. 【丢失更新】查询有网络往返（最长 20s），期间真实业务请求可能撞 429 写入一条更新、
//     更长的限流。解除一律走 ClearModelRateLimitIfObserved：WHERE 精确匹配发起查询
//     【之前】观测到的那一代 rate_limit_reset_at，代际变了就什么都不做。绝不用裸删。
//
//  2. 【只动模型级】只读写 accounts.extra.model_rate_limits，绝不碰 rate_limit_reset_at /
//     rate_limited_at / overload_until —— 账号级限流与 529 过载各有各的写入方与自愈路径。
//
//  3. 【不同池不能互相推翻】主池（rate_limit）健康不能用来解除 spark 的限流，反之亦然。
//     模型 → 池的映射在 model_rate_limit.go 的 openAIDedicatedQuotaPoolMeteredFeatures。
//
//  4. 【不新增定时任务】整条路径只挂在已有的用量刷新上（OpenAIQuotaService.QueryUsage
//     与 AccountUsageService.getOpenAIUsage）。这个仓库刻意没有用量相关的 ticker。

const (
	// openAIModelUsageUnavailableReason 标记这条模型级限流来自 model_usage 的显式
	// available=false，而不是某次 429 的反推。
	openAIModelUsageUnavailableReason = "openai_model_usage_unavailable"

	// openAIModelUsageFallbackCooldown 是上游说"不可用"但没给 available_at 时的冷却时长。
	//
	// 刻意取得很短：上游只说了"现在不行"，没说"到什么时候"，凭这种信息锁几个小时正是
	// 本文件开头那个事故的形态。取 10 分钟等于"下一轮用量刷新时重新判定"，猜错的代价
	// 最多是十分钟内少一个候选账号。
	openAIModelUsageFallbackCooldown = 10 * time.Minute

	// openAIModelUsageMaxCooldown 给 available_at 封顶。上游最长的窗口就是 7d，
	// 超过这个长度的恢复时间只可能是脏数据，宁可早解除一次再被 429 重新锁上，
	// 也不能让一个模型被锁到某个荒谬的未来时间点。
	openAIModelUsageMaxCooldown = 7 * 24 * time.Hour

	// openAIModelSelfHealMinInterval 是用量刷新顺带触发 /wham/usage 的最小间隔，
	// 与 codex 探针的缓存 TTL 对齐，避免把用量刷新变成对上游的定期轮询。
	openAIModelSelfHealMinInterval = openAIProbeCacheTTL
)

// OpenAIModelRateLimitRecoveryRepository 是模型级自愈用的条件清除原语（AccountRepository
// 之外的可选接口，与账号级的 OpenAIRateLimitRecoveryRepository 同构）。
//
// 导出是刻意的：自愈通过类型断言取得它，断言失败只打一条 Debug 就悄悄什么都不做。
// 导出后 repository 侧可以写编译期断言（account_repo_test.go），真实仓库一旦不再满足
// 这个接口就编译失败，而不是让模型级自愈静默消失。
type OpenAIModelRateLimitRecoveryRepository interface {
	ClearModelRateLimitIfObserved(ctx context.Context, id int64, scope string, observedResetAt time.Time) (bool, error)
}

// openAIModelRateLimitObservation 是发起上游查询【之前】拍下的模型级限流快照。
// limits 为空表示这个账号现在没有任何生效中的模型级限流（自愈无事可做，但
// model_usage 的"显式不可用"仍然可以据此写入新的限流）。
type openAIModelRateLimitObservation struct {
	accountID int64
	limits    map[string]time.Time
}

// openAIModelUsageVerdict 是 model_usage 对某个模型的显式表态。
// available 为 nil 表示上游根本没提这个模型（稀疏表的"没信息"）。
type openAIModelUsageVerdict struct {
	model       string
	available   *bool
	availableAt *time.Time
}

// openAIModelRateLimitClear 是一次"解除某个模型的限流"的计划。
// observedResetAt 就是条件清除要匹配的那一代。
type openAIModelRateLimitClear struct {
	scope           string
	observedResetAt time.Time
	evidence        string
}

// openAIModelRateLimitBlock 是一次"按 model_usage 写入/延长限流"的计划。
type openAIModelRateLimitBlock struct {
	scope    string
	resetAt  time.Time
	evidence string
}

// observeModelRateLimitsForSelfHeal 在上游往返之前读一次账号，拍下模型级限流的代际快照。
// 返回 nil 表示这个账号不参与模型级自愈。
//
// 影子账号直接排除：spark 影子整行只承接 bengalfox 这一道额度，它的限流状态完全由
// QueryUsage 自己维护，而这里拿到的 /wham/usage payload 其实描述的是**母账号**；
// 拿母账号的表态去改影子行的模型级限流是跨行写入，风险远大于收益。
func (s *OpenAIQuotaService) observeModelRateLimitsForSelfHeal(ctx context.Context, accountID int64, now time.Time) *openAIModelRateLimitObservation {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil
	}
	if !account.IsOpenAIOAuth() || account.IsShadow() {
		return nil
	}
	return &openAIModelRateLimitObservation{
		accountID: accountID,
		limits:    account.activeModelRateLimits(now),
	}
}

// applyOpenAIModelUsageSelfHeal 把 /wham/usage 的表态落到 accounts.extra.model_rate_limits。
// 任何一步失败都只记日志：用量查询本身已经成功，不能因为自愈失败而让调用方拿不到数据。
func (s *OpenAIQuotaService) applyOpenAIModelUsageSelfHeal(
	ctx context.Context,
	observation *openAIModelRateLimitObservation,
	usage *OpenAIQuotaUsage,
	now time.Time,
) {
	if s == nil || s.accountRepo == nil || observation == nil || usage == nil {
		return
	}
	clears, blocks := planOpenAIModelRateLimitSelfHeal(observation.limits, usage, now)
	if len(clears) == 0 && len(blocks) == 0 {
		return
	}

	if len(clears) > 0 {
		recoveryRepo, ok := s.accountRepo.(OpenAIModelRateLimitRecoveryRepository)
		if !ok {
			slog.Debug("openai_model_rate_limit_self_heal_unsupported",
				"account_id", observation.accountID,
				"reason", "account repository does not implement ClearModelRateLimitIfObserved")
		} else {
			for _, clear := range clears {
				cleared, err := recoveryRepo.ClearModelRateLimitIfObserved(ctx, observation.accountID, clear.scope, clear.observedResetAt)
				switch {
				case err != nil:
					slog.Warn("openai_model_rate_limit_self_heal_failed",
						"account_id", observation.accountID,
						"scope", clear.scope,
						"error", err)
				case !cleared:
					// 往返期间限流代际变了（真实 429 写入了新限流，或已被别处清除/重新武装）。
					// 保持现状，绝不退回无条件删除。
					slog.Info("openai_model_rate_limit_self_heal_generation_changed",
						"account_id", observation.accountID,
						"scope", clear.scope,
						"observed_reset_at", clear.observedResetAt.UTC(),
						"reason", "model rate limit generation changed while /wham/usage was in flight, skipping clear")
				default:
					slog.Info("openai_model_rate_limit_cleared_by_usage_snapshot",
						"account_id", observation.accountID,
						"scope", clear.scope,
						"observed_reset_at", clear.observedResetAt.UTC(),
						"evidence", clear.evidence)
				}
			}
		}
	}

	for _, block := range blocks {
		if err := s.accountRepo.SetModelRateLimit(ctx, observation.accountID, block.scope, block.resetAt, openAIModelUsageUnavailableReason); err != nil {
			slog.Warn("openai_model_rate_limit_self_heal_block_failed",
				"account_id", observation.accountID,
				"scope", block.scope,
				"reset_at", block.resetAt.UTC(),
				"error", err)
			continue
		}
		slog.Info("openai_model_rate_limited_by_usage_snapshot",
			"account_id", observation.accountID,
			"scope", block.scope,
			"reset_at", block.resetAt.UTC(),
			"evidence", block.evidence)
	}
}

// planOpenAIModelRateLimitSelfHeal 是纯判定：给定"查询前观测到的模型级限流"和一份
// /wham/usage 快照，算出该解除哪些、该写入/延长哪些。不碰任何 IO，便于把全部边界
// 情况钉死在单测里。
//
// 证据优先级（从强到弱）：
//
//  1. model_usage 显式 available=false —— 最强的负面信号，任何情况下都不解除；
//  2. 独立额度池模型：只认它自己那个池的条目。池条目在场时，池说了算（健康才解除，
//     不健康则连 model_usage 的 available=true 都不能推翻它）；池条目整个缺席时，
//     才允许退回 model_usage 的显式 available=true；
//  3. 普通模型：model_usage 显式 available=true 才解除；
//  4. 其余一律不动。
func planOpenAIModelRateLimitSelfHeal(
	observed map[string]time.Time,
	usage *OpenAIQuotaUsage,
	now time.Time,
) ([]openAIModelRateLimitClear, []openAIModelRateLimitBlock) {
	if usage == nil {
		return nil, nil
	}
	verdicts := openAIModelUsageVerdicts(usage)

	scopes := make([]string, 0, len(observed))
	for scope := range observed {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)

	// 规范拼写 → 已有 scope：model_usage 的模型名与库里存的 scope 可能拼写不同
	// （大小写、openai/ 前缀、日期后缀），延长限流时要落回同一条，别写出第二条。
	observedByCanonical := make(map[string]string, len(scopes))
	for _, scope := range scopes {
		canonical := openAIModelSelfHealCanonicalKey(scope)
		if canonical == "" {
			continue
		}
		if _, exists := observedByCanonical[canonical]; !exists {
			observedByCanonical[canonical] = scope
		}
	}

	var clears []openAIModelRateLimitClear
	for _, scope := range scopes {
		if openAIModelSelfHealSkipsScope(scope) {
			continue
		}
		verdict, hasVerdict := verdicts[openAIModelSelfHealCanonicalKey(scope)]
		if hasVerdict && verdict.explicitlyUnavailable() {
			continue
		}
		explicitlyAvailable := hasVerdict && verdict.explicitlyAvailable()

		if feature := openAIDedicatedQuotaPoolMeteredFeatureFor(scope); feature != "" {
			healthy, known := openAIDedicatedQuotaPoolHealth(usage, feature)
			switch {
			case known && healthy:
				clears = append(clears, openAIModelRateLimitClear{
					scope:           scope,
					observedResetAt: observed[scope],
					evidence:        "dedicated_quota_pool_healthy:" + feature,
				})
			case known:
				// 池条目在场但没说健康：主池、model_usage 都没有资格替它背书。
			case explicitlyAvailable:
				clears = append(clears, openAIModelRateLimitClear{
					scope:           scope,
					observedResetAt: observed[scope],
					evidence:        "model_usage_available_without_pool_entry:" + feature,
				})
			}
			continue
		}

		if explicitlyAvailable {
			clears = append(clears, openAIModelRateLimitClear{
				scope:           scope,
				observedResetAt: observed[scope],
				evidence:        "model_usage_available",
			})
		}
	}

	canonicals := make([]string, 0, len(verdicts))
	for canonical := range verdicts {
		canonicals = append(canonicals, canonical)
	}
	sort.Strings(canonicals)

	var blocks []openAIModelRateLimitBlock
	for _, canonical := range canonicals {
		verdict := verdicts[canonical]
		if !verdict.explicitlyUnavailable() {
			continue
		}
		scope := verdict.model
		if existingScope, ok := observedByCanonical[canonical]; ok {
			scope = existingScope
		}
		if openAIModelSelfHealSkipsScope(scope) {
			continue
		}

		resetAt := now.Add(openAIModelUsageFallbackCooldown)
		evidence := "model_usage_unavailable"
		if verdict.availableAt != nil && verdict.availableAt.After(now) {
			resetAt = verdict.availableAt.UTC()
			evidence = "model_usage_unavailable_with_available_at"
		}
		if maxResetAt := now.Add(openAIModelUsageMaxCooldown); resetAt.After(maxResetAt) {
			resetAt = maxResetAt
			evidence += "_clamped"
		}
		// 绝不缩短已有的限流：那条可能是一次真实 429 写下的、更可信的冷却。
		if existing, ok := observed[scope]; ok && !existing.Before(resetAt) {
			continue
		}
		blocks = append(blocks, openAIModelRateLimitBlock{
			scope:    scope,
			resetAt:  resetAt,
			evidence: evidence,
		})
	}

	return clears, blocks
}

// explicitlyAvailable 报告上游是否显式说了"可用"（而不是没提）。
func (v openAIModelUsageVerdict) explicitlyAvailable() bool {
	return v.available != nil && *v.available
}

// explicitlyUnavailable 报告上游是否显式说了"不可用"（而不是没提）。
func (v openAIModelUsageVerdict) explicitlyUnavailable() bool {
	return v.available != nil && !*v.available
}

// openAIModelUsageVerdicts 把稀疏的 model_usage 整理成"规范拼写 → 显式表态"。
// 没有 available 字段的条目照样收进来（available 保持 nil），这样调用方读到的就是
// 上游的原意："这个模型被提到了，但没说能不能用"。
func openAIModelUsageVerdicts(usage *OpenAIQuotaUsage) map[string]openAIModelUsageVerdict {
	if usage == nil || len(usage.ModelUsage) == 0 {
		return nil
	}
	models := make([]string, 0, len(usage.ModelUsage))
	for model := range usage.ModelUsage {
		models = append(models, model)
	}
	sort.Strings(models)

	verdicts := make(map[string]openAIModelUsageVerdict, len(models))
	for _, model := range models {
		name := strings.TrimSpace(model)
		canonical := openAIModelSelfHealCanonicalKey(name)
		if canonical == "" {
			continue
		}
		entry := usage.ModelUsage[model]
		verdict := openAIModelUsageVerdict{model: name, available: entry.Available}
		if availableAt, ok := entry.AvailableAtTime(); ok {
			verdict.availableAt = &availableAt
		}
		// 多个拼写归一到同一个规范名时，负面信号优先；其余保留先到的（models 已排序，
		// 结果是确定的）。
		if existing, exists := verdicts[canonical]; exists &&
			(existing.explicitlyUnavailable() || !verdict.explicitlyUnavailable()) {
			continue
		}
		verdicts[canonical] = verdict
	}
	if len(verdicts) == 0 {
		return nil
	}
	return verdicts
}

// openAIDedicatedQuotaPoolHealth 读 additional_rate_limits 里某个池的健康状况。
//
// known=false 表示"这次响应里根本没有这个池的可用信息"，与"池不健康"是两回事：
// 前者允许退回更弱的证据，后者不允许任何人推翻。
//
// healthy 的条件刻意收得很紧：allowed=true、limit_reached=false，**且两个窗口都在场、
// 都 < 100%**。窗口缺席按"不健康"处理（known=true, healthy=false）——没有余量数字就
// 没有正面证据，此时继续锁到 reset_at 只是回到没有自愈时的老行为，是安全的一侧。
func openAIDedicatedQuotaPoolHealth(usage *OpenAIQuotaUsage, meteredFeature string) (healthy bool, known bool) {
	feature := strings.TrimSpace(meteredFeature)
	if usage == nil || feature == "" {
		return false, false
	}
	for i := range usage.AdditionalRateLimits {
		entry := usage.AdditionalRateLimits[i]
		if !strings.EqualFold(strings.TrimSpace(entry.MeteredFeature), feature) {
			continue
		}
		limit := entry.RateLimit
		if limit == nil {
			// 条目在、额度信封缺席：等于没告诉我们任何余量，按"没信息"处理。
			return false, false
		}
		if !limit.Allowed || limit.LimitReached {
			return false, true
		}
		if limit.PrimaryWindow == nil || limit.SecondaryWindow == nil {
			return false, true
		}
		if limit.PrimaryWindow.UsedPercent >= 100 || limit.SecondaryWindow.UsedPercent >= 100 {
			return false, true
		}
		return true, true
	}
	return false, false
}

// openAIModelSelfHealCanonicalKey 归一化模型名/scope，用于把上游的 model_usage 键与
// 库里存的 scope 对上。归一化失败时退回小写原名，保证不同拼写至少能自洽比较。
func openAIModelSelfHealCanonicalKey(model string) string {
	name := strings.TrimSpace(model)
	if name == "" {
		return ""
	}
	if canonical := canonicalizeOpenAIModelAliasSpelling(name); canonical != "" {
		return canonical
	}
	return strings.ToLower(name)
}

// openAIModelSelfHealSkipsScope 过滤掉 model_rate_limits 里那些不是上游模型名的
// 命名空间 key（openai:image_generation、antigravity:gemini 等）。它们由各自的路径
// 写入与清除，/wham/usage 对它们没有任何表态。
func openAIModelSelfHealSkipsScope(scope string) bool {
	return strings.TrimSpace(scope) == "" || strings.Contains(scope, ":")
}

// refreshOpenAIModelRateLimitSelfHeal 让"用量刷新"顺带带动模型级自愈。
//
// 为什么必须在这里额外发一次 /wham/usage：普通账号的用量刷新走的是 /responses 探针，
// 那条链只有 x-codex-* 头、不带池名，读不到 model_usage 也读不到 codex_bengalfox 的余量。
// 不接这一脚的话，模型级自愈就只能等管理员手动打开额度卡片才会发生 —— 而事故里
// 受苦的是客户端，不是管理员。
//
// 触发条件卡得很紧，避免把用量刷新变成对上游的轮询（本仓库刻意没有用量相关的 ticker）：
//
//   - 仅 OpenAI OAuth 非影子账号；
//   - 账号当前**确实**有生效中的模型级限流（没有限流就没有要自愈的东西）；
//   - 每账号至少间隔 openAIModelSelfHealMinInterval（10 分钟），force 刷新可绕过。
//
// 真正的判定与写库都在 QueryUsage 内部完成（它自己会在往返前拍代际快照），
// 这里只负责决定"要不要发起这次查询"。
func (s *AccountUsageService) refreshOpenAIModelRateLimitSelfHeal(ctx context.Context, account *Account, now time.Time, force bool) {
	if s == nil || s.openAIQuotaService == nil || account == nil {
		return
	}
	if !account.IsOpenAIOAuth() || account.IsShadow() {
		return
	}
	if len(account.activeModelRateLimits(now)) == 0 {
		return
	}
	if !s.shouldSelfHealOpenAIModelRateLimits(account.ID, now, force) {
		return
	}
	if _, err := s.openAIQuotaService.QueryUsage(ctx, account.ID); err != nil {
		// 上游查不到就什么都不做：没有证据 ⇒ 不解除，限流按原样保留到 reset_at。
		slog.Debug("openai_model_rate_limit_self_heal_usage_query_failed",
			"account_id", account.ID,
			"error", err)
	}
}

// shouldSelfHealOpenAIModelRateLimits 是每账号的节流闸门，语义与
// shouldProbeOpenAICodexSnapshot 一致：返回 true 的同时就记下这次的时间戳。
func (s *AccountUsageService) shouldSelfHealOpenAIModelRateLimits(accountID int64, now time.Time, force bool) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	if !force {
		if cached, ok := s.openAIModelSelfHealAt.Load(accountID); ok {
			if ts, ok := cached.(time.Time); ok && now.Sub(ts) < openAIModelSelfHealMinInterval {
				return false
			}
		}
	}
	s.openAIModelSelfHealAt.Store(accountID, now)
	return true
}
