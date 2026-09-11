package service

import (
	"context"
	"log/slog"
	"sort"
)

// 运维逃生口：无条件清空网关进程内存里的调度封锁状态。
//
// 为什么必须有这么一个入口：这些封锁只活在正在跑的网关进程的内存里。外面再起一个
// 进程（CLI、脚本、另一个容器）都碰不到它们，后台账号页读的又只是数据库，于是内存
// 封锁在页面上完全不可见——账号看起来一切正常，实际一个请求都不接。
//
// 2026-09-11 公司实例上就是这样：账号 5、6 的数据库记录干干净净（active、
// schedulable=true，没有限流、没有过载、没有临时停调、没过期），可它们就是不接单。
// 原因是被封在内存里了。
//
// 更要命的是，现成的 RateLimitService.RecoverAccountState 恰恰在这种情况下没用：
// 它解除内存封锁的那一行 notifyAccountSchedulingBlockCleared 被包在
// if result.ClearedError && !result.ClearedRateLimit 里，而库里本来就干净时
// ClearedError 和 ClearedRateLimit 都是 false，两个分支都进不去。于是后台的「测试」
// 和「恢复状态」按钮在最需要它们的时候什么也没做，只剩重启容器一条路——而重启会
// 掐断所有在途请求。
//
// 所以这个入口的全部意义就是「无条件」：不读数据库、不看账号状态、不做任何前置判断，
// 调用即清。任何"先检查再清"的改动都会让它退化成又一个 RecoverAccountState。

// 作用域取值：不指定账号 = all（全量），指定账号 = accounts。
const (
	AccountRuntimeBlocksResetScopeAll      = "all"
	AccountRuntimeBlocksResetScopeAccounts = "accounts"
)

// AccountRuntimeBlockClearCounts 是每类内存状态**真正删掉的条目数**。
// 没删到就是 0，绝不把"跳过"粉饰成"清掉了"。
//
// ProxyQuarantines 与 GrokTeamRateLimits 在"指定账号"模式下恒为 0：前者按 proxy_id
// 索引，后者按 team 指纹索引，都无法从账号 ID 精确反查——而反查要读库，正好是这个
// 逃生口不该依赖的东西。详见各自的清除函数注释。
type AccountRuntimeBlockClearCounts struct {
	AccountRuntimeBlocks    int `json:"account_runtime_blocks"`
	ModelTransientCooldowns int `json:"model_transient_cooldowns"`
	ProxyQuarantines        int `json:"proxy_quarantines"`
	GrokModelQuotaBlocks    int `json:"grok_model_quota_blocks"`
	GrokTeamRateLimits      int `json:"grok_team_rate_limits"`
	GrokFreeQuotaGates      int `json:"grok_free_quota_gates"`
}

func (c AccountRuntimeBlockClearCounts) total() int {
	return c.AccountRuntimeBlocks +
		c.ModelTransientCooldowns +
		c.ProxyQuarantines +
		c.GrokModelQuotaBlocks +
		c.GrokTeamRateLimits +
		c.GrokFreeQuotaGates
}

// AccountRuntimeBlocksResetResult 是接口返回体。AccountIDs 始终非 nil，全量模式下
// 序列化成 []（而不是 null），免得调用方要分两种情况解析。
type AccountRuntimeBlocksResetResult struct {
	Scope        string                         `json:"scope"`
	AccountIDs   []int64                        `json:"account_ids"`
	Cleared      AccountRuntimeBlockClearCounts `json:"cleared"`
	TotalCleared int                            `json:"total_cleared"`
}

// ClearAccountRuntimeBlocks 无条件抹掉进程内存里的全部调度封锁。
//
// accountIDs 为空（nil 或空数组）表示全量；否则只清列出的账号能精确定位到的那几类。
// 接收方已经过归一化（去重、丢掉非正数、升序），返回体里回显的就是归一化后的结果。
//
// 接收者可以为 nil：Grok 的三张表是包级全局变量，没有网关实例照样能清；只有整号停调、
// 账号×模型冷却、代理熔断这三类挂在实例上，实例缺席时它们本来也不存在。
func (s *OpenAIGatewayService) ClearAccountRuntimeBlocks(ctx context.Context, accountIDs []int64) AccountRuntimeBlocksResetResult {
	if ctx == nil {
		ctx = context.Background()
	}
	ids := normalizeAccountRuntimeBlockAccountIDs(accountIDs)
	scoped := len(ids) > 0

	result := AccountRuntimeBlocksResetResult{
		Scope:      AccountRuntimeBlocksResetScopeAll,
		AccountIDs: ids,
	}
	if scoped {
		result.Scope = AccountRuntimeBlocksResetScopeAccounts
	}

	result.Cleared.AccountRuntimeBlocks = s.clearOpenAIAccountRuntimeBlocks(ids)
	result.Cleared.ModelTransientCooldowns = s.getOpenAIAccountModelTransientState().clearBlocks(ids)
	result.Cleared.GrokModelQuotaBlocks = clearGrokModelQuotaBlocks(ids)
	result.Cleared.GrokFreeQuotaGates = s.clearGrokFreeQuotaGateCaches(ctx, ids)
	if !scoped {
		// 这两类的键里没有账号维度，只有全量模式才敢动。
		result.Cleared.ProxyQuarantines = s.getOpenAIProxyStreamCircuit().clearAll()
		result.Cleared.GrokTeamRateLimits = clearAllGrokTeamModelRateLimits()
	}
	result.TotalCleared = result.Cleared.total()

	// 这是人为的运维干预，出事后要能在日志里对得上时间点，所以用 Warn 而不是 Info。
	slog.Warn("account_runtime_blocks_cleared",
		"scope", result.Scope,
		"account_ids", result.AccountIDs,
		"account_runtime_blocks", result.Cleared.AccountRuntimeBlocks,
		"model_transient_cooldowns", result.Cleared.ModelTransientCooldowns,
		"proxy_quarantines", result.Cleared.ProxyQuarantines,
		"grok_model_quota_blocks", result.Cleared.GrokModelQuotaBlocks,
		"grok_team_rate_limits", result.Cleared.GrokTeamRateLimits,
		"grok_free_quota_gates", result.Cleared.GrokFreeQuotaGates,
		"total_cleared", result.TotalCleared,
	)
	return result
}

// clearGrokFreeQuotaGateCaches 清掉 Grok free 额度软闸的观测缓存。
//
// 这张缓存存的是"该账号最近窗口用了多少 token"，够了阈值就在调度时把账号过滤掉。
// 它按 accountID 建键，所以按账号精确清除是成立的。
//
// 有三份：Gateway 选号路径一份、OpenAI 选号路径一份（都是包级全局），高级调度器开启时
// 还有挂在 defaultOpenAIAccountScheduler 实例上的第三份。第三份只能通过调度器实例拿到；
// 高级调度器没开时 getOpenAIAccountScheduler 返回 nil，那份缓存也不会有人读，跳过即可。
//
// 注意：清空只是让下一次请求重新去查（查回来之前一律放行）。如果此刻正好有一个后台
// 刷新在飞，它落地时会重新写入一条真实的观测值——那是数据库里真实的用量，本来就该生效，
// 不属于"清不掉的内存幽灵"。
func (s *OpenAIGatewayService) clearGrokFreeQuotaGateCaches(ctx context.Context, accountIDs []int64) int {
	cleared := clearGrokFreeQuotaGateCache(&gatewayGrokFreeQuotaGateCache, accountIDs)
	cleared += clearGrokFreeQuotaGateCache(&openaiGrokFreeQuotaGateCache, accountIDs)
	if s == nil {
		return cleared
	}
	if scheduler, ok := s.getOpenAIAccountScheduler(ctx).(*defaultOpenAIAccountScheduler); ok && scheduler != nil {
		cleared += clearGrokFreeQuotaGateCache(&scheduler.grokFreeQuotaGateCache, accountIDs)
	}
	return cleared
}

// normalizeAccountRuntimeBlockAccountIDs 去重、丢掉非正数、升序排列。
// 永远返回非 nil 切片，空输入得到空切片而不是 nil。
func normalizeAccountRuntimeBlockAccountIDs(accountIDs []int64) []int64 {
	out := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
