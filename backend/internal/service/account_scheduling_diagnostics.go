package service

import (
	"context"
	"sort"
	"time"
)

// Scheduling block sources reported by DiagnoseAccountScheduling.
const (
	// DB-derived: mirror (*Account).IsSchedulable().
	AccountSchedulingBlockStatus              = "status"
	AccountSchedulingBlockManualUnschedulable = "manual_unschedulable"
	AccountSchedulingBlockExpired             = "expired"
	AccountSchedulingBlockOverloaded          = "overloaded"
	AccountSchedulingBlockRateLimited         = "rate_limited"
	AccountSchedulingBlockTempUnschedulable   = "temp_unschedulable"
	AccountSchedulingBlockQuotaExceeded       = "quota_exceeded"

	// In-process gateway state (OpenAI / Grok accounts only).
	AccountSchedulingBlockRuntime         = "runtime_block"
	AccountSchedulingBlockModelRuntime    = "model_runtime_block"
	AccountSchedulingBlockProxyQuarantine = "proxy_quarantine"
	AccountSchedulingBlockQuotaAutoPause  = "quota_auto_pause"

	// In-process Grok-only state. Both are model-scoped: they exclude the account
	// for one model, not entirely.
	AccountSchedulingBlockGrokModelQuota    = "grok_model_quota"
	AccountSchedulingBlockGrokTeamRateLimit = "grok_team_rate_limit"
)

// modelScopedSchedulingBlockSources 只挡单个模型、不影响整号可调度性的封锁来源。
// 新增模型级来源时必须同步加进来，否则账号会被误判成整号不可调度。
var modelScopedSchedulingBlockSources = map[string]struct{}{
	AccountSchedulingBlockModelRuntime:      {},
	AccountSchedulingBlockGrokModelQuota:    {},
	AccountSchedulingBlockGrokTeamRateLimit: {},
}

// AccountSchedulingBlock is one reason the scheduler currently skips an account.
// Only the fields relevant to Source are populated.
type AccountSchedulingBlock struct {
	Source      string     `json:"source"`
	Until       *time.Time `json:"until,omitempty"`
	Status      string     `json:"status,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	Model       string     `json:"model,omitempty"`
	ProxyID     int64      `json:"proxy_id,omitempty"`
	Window      string     `json:"window,omitempty"`
	Threshold   float64    `json:"threshold,omitempty"`
	Utilization float64    `json:"utilization,omitempty"`
}

// AccountSchedulingDiagnosis explains whether the scheduler would currently
// consider an account at all. Schedulable is false when any account-wide block
// is present; model-scoped cooldowns (see modelScopedSchedulingBlockSources)
// only exclude the account for that model, so they are listed but leave
// Schedulable true.
type AccountSchedulingDiagnosis struct {
	Schedulable bool                     `json:"schedulable"`
	Blocks      []AccountSchedulingBlock `json:"blocks"`
}

// DiagnoseAccountScheduling reports, per account, every condition that keeps
// the scheduler from selecting it: the persisted conditions checked by
// (*Account).IsSchedulable() plus the gateway's in-process blocks (account
// runtime block, account+model transient cooldown, proxy stream quarantine,
// quota auto-pause, and for Grok the free-usage model block and the team-wide
// model rate limit).
//
// This is the single source of truth for "can this account take traffic right
// now" across the admin UI — the account list and the ops availability card
// both read it. Do not re-derive availability from raw DB columns somewhere
// else; that is exactly how those call sites drifted apart before, reporting
// accounts as available while the gateway refused to schedule them.
//
// In-process state is per instance: in a multi-instance deployment the result
// reflects only the instance that served the admin request.
//
// The diagnosis is strictly read-only. The scheduler's own checks
// (isOpenAIAccountRuntimeBlocked, the transient/circuit isBlocked helpers)
// expire entries, bump generations and refresh LRU timestamps as a side
// effect; this path uses dedicated peeks that never mutate that state.
//
// A nil receiver is allowed and yields the persisted (DB-derived) blocks only.
func (s *OpenAIGatewayService) DiagnoseAccountScheduling(ctx context.Context, accounts []*Account) map[int64]AccountSchedulingDiagnosis {
	out := make(map[int64]AccountSchedulingDiagnosis, len(accounts))
	if len(accounts) == 0 {
		return out
	}
	now := time.Now()

	var (
		quotaCtx    context.Context
		modelBlocks map[int64][]openAIAccountModelTransientBlock
		circuit     *openAIProxyStreamCircuit
	)
	if s != nil {
		quotaCtx = s.withOpenAIQuotaAutoPauseContext(ctx)
		openAIIDs := make([]int64, 0, len(accounts))
		for _, account := range accounts {
			if isOpenAIAccount(account) {
				openAIIDs = append(openAIIDs, account.ID)
			}
		}
		if len(openAIIDs) > 0 {
			modelBlocks = s.getOpenAIAccountModelTransientState().activeBlocks(openAIIDs, now)
			circuit = s.getOpenAIProxyStreamCircuit()
		}
	}

	for _, account := range accounts {
		if account == nil {
			continue
		}
		blocks := diagnoseAccountPersistedSchedulingBlocks(account, now)
		if s != nil && isOpenAIAccount(account) {
			if until, ok := s.peekOpenAIAccountRuntimeBlockUntil(account.ID); ok && now.Before(until) {
				blocks = append(blocks, AccountSchedulingBlock{
					Source: AccountSchedulingBlockRuntime,
					Until:  diagnosisTimePtr(until),
				})
			}
			for _, block := range modelBlocks[account.ID] {
				blocks = append(blocks, AccountSchedulingBlock{
					Source: AccountSchedulingBlockModelRuntime,
					Model:  block.model,
					Until:  diagnosisTimePtr(block.blockUntil),
				})
			}
			// The scheduler re-admits quarantined proxies in a fail-open second
			// pass when nothing else is available, so this block is soft.
			if proxyID, ok := openAIProxyStreamCircuitProxyID(account); ok {
				if until, blocked := circuit.peekBlockedUntil(proxyID, now); blocked {
					blocks = append(blocks, AccountSchedulingBlock{
						Source:  AccountSchedulingBlockProxyQuarantine,
						ProxyID: proxyID,
						Until:   diagnosisTimePtr(until),
					})
				}
			}
			if block, ok := diagnoseAccountQuotaAutoPause(quotaCtx, account); ok {
				blocks = append(blocks, block)
			}
			blocks = appendGrokModelScopedBlocks(blocks, account, now)
		}
		out[account.ID] = AccountSchedulingDiagnosis{
			Schedulable: !hasAccountWideSchedulingBlock(blocks),
			Blocks:      blocks,
		}
	}
	return out
}

func hasAccountWideSchedulingBlock(blocks []AccountSchedulingBlock) bool {
	for _, block := range blocks {
		if _, modelScoped := modelScopedSchedulingBlockSources[block.Source]; !modelScoped {
			return true
		}
	}
	return false
}

// diagnoseAccountPersistedSchedulingBlocks mirrors (*Account).IsSchedulable(),
// emitting one block per failing condition instead of short-circuiting.
func diagnoseAccountPersistedSchedulingBlocks(a *Account, now time.Time) []AccountSchedulingBlock {
	blocks := make([]AccountSchedulingBlock, 0, 2)
	if !a.IsActive() {
		blocks = append(blocks, AccountSchedulingBlock{Source: AccountSchedulingBlockStatus, Status: a.Status})
	}
	if !a.Schedulable {
		blocks = append(blocks, AccountSchedulingBlock{Source: AccountSchedulingBlockManualUnschedulable})
	}
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		blocks = append(blocks, AccountSchedulingBlock{Source: AccountSchedulingBlockExpired})
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		blocks = append(blocks, AccountSchedulingBlock{
			Source: AccountSchedulingBlockOverloaded,
			Until:  diagnosisTimePtr(*a.OverloadUntil),
		})
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		blocks = append(blocks, AccountSchedulingBlock{
			Source: AccountSchedulingBlockRateLimited,
			Until:  diagnosisTimePtr(*a.RateLimitResetAt),
		})
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		blocks = append(blocks, AccountSchedulingBlock{
			Source: AccountSchedulingBlockTempUnschedulable,
			Until:  diagnosisTimePtr(*a.TempUnschedulableUntil),
			Reason: a.TempUnschedulableReason,
		})
	}
	if a.IsAPIKeyOrBedrock() && a.IsQuotaExceeded() {
		blocks = append(blocks, AccountSchedulingBlock{Source: AccountSchedulingBlockQuotaExceeded})
	}
	return blocks
}

// appendGrokModelScopedBlocks 补上两处 Grok 专属的进程内单模型封锁：账号自己的
// 免费额度软封（filterGrokModelQuotaBlockedAccounts 消费）与 team 维度的连坐限流
// （filterGrokTeamModelRateLimitedAccounts 消费）。两者此前任何页面都看不到。
//
// 模型名排序输出，避免 map 遍历顺序让同一状态每次刷新都换个排法。
//
// 未覆盖：Grok free 账号的 95% 本地软闸（grok_free_quota_gate.go）。它要查用量
// 统计才能判定，不是纯内存读，塞进这条只读路径会带上 I/O 和缓存副作用。
func appendGrokModelScopedBlocks(blocks []AccountSchedulingBlock, account *Account, now time.Time) []AccountSchedulingBlock {
	if account == nil || account.Platform != PlatformGrok {
		return blocks
	}
	appendSorted := func(source string, entries map[string]time.Time) {
		if len(entries) == 0 {
			return
		}
		models := make([]string, 0, len(entries))
		for model := range entries {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			blocks = append(blocks, AccountSchedulingBlock{
				Source: source,
				Model:  model,
				Until:  diagnosisTimePtr(entries[model]),
			})
		}
	}
	appendSorted(AccountSchedulingBlockGrokModelQuota, peekGrokModelQuotaBlocks(account.ID, now))
	appendSorted(AccountSchedulingBlockGrokTeamRateLimit, peekGrokTeamModelRateLimits(account, now))
	return blocks
}

// diagnoseAccountQuotaAutoPause mirrors the scheduler's quota auto-pause gate
// (OpenAI codex windows and Grok quota snapshots). Both checks only read the
// account snapshot and the cached auto-pause settings carried by ctx.
func diagnoseAccountQuotaAutoPause(ctx context.Context, account *Account) (AccountSchedulingBlock, bool) {
	paused, decision := shouldAutoPauseOpenAIAccountByQuota(ctx, account)
	if !paused {
		paused, decision = shouldAutoPauseGrokAccountByQuota(account)
	}
	if !paused {
		return AccountSchedulingBlock{}, false
	}
	return AccountSchedulingBlock{
		Source:      AccountSchedulingBlockQuotaAutoPause,
		Window:      decision.window,
		Threshold:   decision.threshold,
		Utilization: decision.utilization,
	}, true
}

func diagnosisTimePtr(t time.Time) *time.Time {
	utc := t.UTC()
	return &utc
}
