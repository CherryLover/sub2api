//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newRuntimeBlockResetAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func newGrokRuntimeBlockResetAccount(id int64, teamID string) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"team_id": teamID},
	}
}

// resetRuntimeBlockResetGlobals 把包级全局的内存封锁表清干净，测试前后各来一次，
// 免得这些进程级状态在同包测试之间互相串味。
func resetRuntimeBlockResetGlobals(t *testing.T) {
	t.Helper()
	reset := func() {
		globalGrokModelQuotaBlocks.mu.Lock()
		globalGrokModelQuotaBlocks.items = make(map[string]grokModelQuotaBlock)
		globalGrokModelQuotaBlocks.mu.Unlock()

		globalGrokTeamModelRateLimits.mu.Lock()
		globalGrokTeamModelRateLimits.items = make(map[string]grokTeamModelRateLimit)
		globalGrokTeamModelRateLimits.mu.Unlock()

		clearGrokFreeQuotaGateCache(&gatewayGrokFreeQuotaGateCache, nil)
		clearGrokFreeQuotaGateCache(&openaiGrokFreeQuotaGateCache, nil)
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
	}
	reset()
	t.Cleanup(reset)
}

// seedModelTransientCooldown 写出一条**真的在冷却中**的账号×模型条目：
// 连续 2 次失败才会进入 10s 冷却，只失败 1 次的条目 blockUntil 是零值。
func seedModelTransientCooldown(t *testing.T, state *openAIAccountModelTransientState, accountID int64, model string, now time.Time) {
	t.Helper()
	state.recordFailure(accountID, model, now)
	decision := state.recordFailure(accountID, model, now.Add(time.Millisecond))
	require.Greater(t, decision.Cooldown, time.Duration(0), "连续两次失败才会进入冷却")
	require.True(t, state.isBlocked(accountID, model, now.Add(2*time.Millisecond)))
}

// seedProxyQuarantine 让某个 proxy 真的被隔离：阈值是 2 次失败，
// 两次之间必须拉开 collapseInterval（3s），否则会被合并成一次事件。
func seedProxyQuarantine(t *testing.T, circuit *openAIProxyStreamCircuit, proxyID int64, now time.Time) {
	t.Helper()
	circuit.recordFailure(proxyID, now)
	tripped, _ := circuit.recordFailure(proxyID, now.Add(10*time.Second))
	require.True(t, tripped, "第二次失败应当触发隔离")
}

// 全量模式：六类内存封锁必须一个不剩，计数必须等于真正删掉的条目数。
//
// 这是 2026-09-11 那次事故真正需要的能力：账号 5、6 的数据库记录干干净净——active、
// schedulable=true，没有限流、没有过载、没有临时停调、也没过期——却一个请求都不接，
// 因为它们被封在内存里，而后台页面只读数据库，什么异常都看不出来。
func TestClearAccountRuntimeBlocks_AllScopeClearsEveryStore(t *testing.T) {
	resetRuntimeBlockResetGlobals(t)
	now := time.Now()
	svc := &OpenAIGatewayService{}

	blocked := newRuntimeBlockResetAccount(5)
	alsoBlocked := newRuntimeBlockResetAccount(6)
	grok := newGrokRuntimeBlockResetAccount(7, "team-alpha")

	svc.BlockAccountScheduling(blocked, now.Add(time.Hour), "test")
	svc.BlockAccountScheduling(alsoBlocked, now.Add(time.Hour), "test")

	transient := svc.getOpenAIAccountModelTransientState()
	seedModelTransientCooldown(t, transient, 5, "gpt-5", now)
	seedModelTransientCooldown(t, transient, 5, "gpt-5-codex", now)
	seedModelTransientCooldown(t, transient, 6, "gpt-5", now)

	circuit := svc.getOpenAIProxyStreamCircuit()
	seedProxyQuarantine(t, circuit, 11, now)

	markGrokModelQuotaBlock(5, "grok-4.5", now.Add(2*time.Hour))
	markGrokModelQuotaBlock(6, "grok-4", now.Add(2*time.Hour))
	markGrokTeamModelRateLimit(grok, "grok-4.5", now.Add(20*time.Minute))
	gatewayGrokFreeQuotaGateCache.Store(int64(5), grokFreeQuotaGateCacheEntry{tokens: 1, checkedAt: now, known: true})
	openaiGrokFreeQuotaGateCache.Store(int64(6), grokFreeQuotaGateCacheEntry{tokens: 2, checkedAt: now, known: true})

	result := svc.ClearAccountRuntimeBlocks(context.Background(), nil)

	require.Equal(t, AccountRuntimeBlocksResetScopeAll, result.Scope)
	require.NotNil(t, result.AccountIDs, "全量模式也要序列化成 []，不能是 null")
	require.Empty(t, result.AccountIDs)
	require.Equal(t, AccountRuntimeBlockClearCounts{
		AccountRuntimeBlocks:    2,
		ModelTransientCooldowns: 3,
		ProxyQuarantines:        1,
		GrokModelQuotaBlocks:    2,
		GrokTeamRateLimits:      1,
		GrokFreeQuotaGates:      2,
	}, result.Cleared)
	require.Equal(t, 11, result.TotalCleared)

	// 计数好看没用，状态得真的空了
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(blocked))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(alsoBlocked))
	require.Zero(t, transient.size())
	require.Zero(t, circuit.activeBlockCount(now.Add(11*time.Second)))
	require.False(t, isGrokModelQuotaBlocked(5, "grok-4.5", now))
	require.False(t, isGrokModelQuotaBlocked(6, "grok-4", now))
	require.False(t, isGrokTeamModelRateLimited(grok, "grok-4.5", now))
	_, ok := gatewayGrokFreeQuotaGateCache.Load(int64(5))
	require.False(t, ok)
	_, ok = openaiGrokFreeQuotaGateCache.Load(int64(6))
	require.False(t, ok)
}

// 指定账号模式：只清这个账号的，别的账号一根汗毛都不能动。
func TestClearAccountRuntimeBlocks_AccountScopeLeavesOtherAccountsAlone(t *testing.T) {
	resetRuntimeBlockResetGlobals(t)
	now := time.Now()
	svc := &OpenAIGatewayService{}

	target := newRuntimeBlockResetAccount(5)
	other := newRuntimeBlockResetAccount(6)

	svc.BlockAccountScheduling(target, now.Add(time.Hour), "test")
	svc.BlockAccountScheduling(other, now.Add(time.Hour), "test")

	transient := svc.getOpenAIAccountModelTransientState()
	seedModelTransientCooldown(t, transient, 5, "gpt-5", now)
	seedModelTransientCooldown(t, transient, 5, "gpt-5-codex", now)
	seedModelTransientCooldown(t, transient, 6, "gpt-5", now)

	markGrokModelQuotaBlock(5, "grok-4.5", now.Add(2*time.Hour))
	markGrokModelQuotaBlock(6, "grok-4.5", now.Add(2*time.Hour))
	gatewayGrokFreeQuotaGateCache.Store(int64(5), grokFreeQuotaGateCacheEntry{tokens: 1, checkedAt: now, known: true})
	gatewayGrokFreeQuotaGateCache.Store(int64(6), grokFreeQuotaGateCacheEntry{tokens: 1, checkedAt: now, known: true})
	openaiGrokFreeQuotaGateCache.Store(int64(5), grokFreeQuotaGateCacheEntry{tokens: 1, checkedAt: now, known: true})

	// 顺带验证入参归一化：重复、0、负数都要被丢掉，回显的是去重升序后的结果
	result := svc.ClearAccountRuntimeBlocks(context.Background(), []int64{5, 5, 0, -3})

	require.Equal(t, AccountRuntimeBlocksResetScopeAccounts, result.Scope)
	require.Equal(t, []int64{5}, result.AccountIDs)
	require.Equal(t, AccountRuntimeBlockClearCounts{
		AccountRuntimeBlocks:    1,
		ModelTransientCooldowns: 2,
		GrokModelQuotaBlocks:    1,
		GrokFreeQuotaGates:      2,
	}, result.Cleared)
	require.Equal(t, 6, result.TotalCleared)

	require.False(t, svc.isOpenAIAccountRuntimeBlocked(target))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(other), "账号 6 不该被牵连")

	require.Equal(t, 1, transient.size(), "只剩账号 6 的那条")
	require.True(t, transient.isBlocked(6, "gpt-5", now.Add(2*time.Millisecond)))
	require.False(t, transient.isBlocked(5, "gpt-5", now.Add(2*time.Millisecond)))
	require.False(t, transient.isBlocked(5, "gpt-5-codex", now.Add(2*time.Millisecond)))

	require.False(t, isGrokModelQuotaBlocked(5, "grok-4.5", now))
	require.True(t, isGrokModelQuotaBlocked(6, "grok-4.5", now), "账号 6 的 Grok 单模型封锁要留着")

	_, ok := gatewayGrokFreeQuotaGateCache.Load(int64(5))
	require.False(t, ok)
	_, ok = openaiGrokFreeQuotaGateCache.Load(int64(5))
	require.False(t, ok)
	_, ok = gatewayGrokFreeQuotaGateCache.Load(int64(6))
	require.True(t, ok, "账号 6 的 free 额度观测要留着")
}

// 按账号精确过滤不了的两类（代理熔断按 proxy_id 索引、team 限流按 team 指纹索引）：
// 指定账号模式下必须跳过、计数如实为 0，而且绝不能顺手误删；全量模式下必须真的清掉。
func TestClearAccountRuntimeBlocks_AccountScopeSkipsProxyAndTeamState(t *testing.T) {
	resetRuntimeBlockResetGlobals(t)
	now := time.Now()
	svc := &OpenAIGatewayService{}
	grok := newGrokRuntimeBlockResetAccount(5, "team-alpha")

	circuit := svc.getOpenAIProxyStreamCircuit()
	seedProxyQuarantine(t, circuit, 11, now)
	markGrokTeamModelRateLimit(grok, "grok-4.5", now.Add(20*time.Minute))

	scoped := svc.ClearAccountRuntimeBlocks(context.Background(), []int64{5})

	require.Zero(t, scoped.Cleared.ProxyQuarantines, "按账号反查不到 proxy_id，只能如实报 0")
	require.Zero(t, scoped.Cleared.GrokTeamRateLimits, "按账号反查不到 team 指纹，只能如实报 0")
	require.Zero(t, scoped.TotalCleared)
	require.Equal(t, 1, circuit.activeBlockCount(now.Add(11*time.Second)), "跳过就是跳过，不许误删")
	require.True(t, isGrokTeamModelRateLimited(grok, "grok-4.5", now), "跳过就是跳过，不许误删")

	all := svc.ClearAccountRuntimeBlocks(context.Background(), nil)

	require.Equal(t, 1, all.Cleared.ProxyQuarantines)
	require.Equal(t, 1, all.Cleared.GrokTeamRateLimits)
	require.Zero(t, circuit.activeBlockCount(now.Add(11*time.Second)))
	require.False(t, isGrokTeamModelRateLimited(grok, "grok-4.5", now))
}

// 什么都没封的时候调用：全 0，不报错，也不许把 account_ids 序列化成 null。
func TestClearAccountRuntimeBlocks_EmptyStateReturnsZeros(t *testing.T) {
	resetRuntimeBlockResetGlobals(t)
	svc := &OpenAIGatewayService{}

	all := svc.ClearAccountRuntimeBlocks(context.Background(), nil)
	require.Equal(t, AccountRuntimeBlocksResetScopeAll, all.Scope)
	require.NotNil(t, all.AccountIDs)
	require.Empty(t, all.AccountIDs)
	require.Equal(t, AccountRuntimeBlockClearCounts{}, all.Cleared)
	require.Zero(t, all.TotalCleared)

	scoped := svc.ClearAccountRuntimeBlocks(context.Background(), []int64{42})
	require.Equal(t, AccountRuntimeBlocksResetScopeAccounts, scoped.Scope)
	require.Equal(t, []int64{42}, scoped.AccountIDs)
	require.Equal(t, AccountRuntimeBlockClearCounts{}, scoped.Cleared)
	require.Zero(t, scoped.TotalCleared)
}

// 逃生口不该因为网关实例没装上就罢工：Grok 那几张表是进程级全局变量，照样清得掉。
func TestClearAccountRuntimeBlocks_NilGatewayStillClearsGlobalStores(t *testing.T) {
	resetRuntimeBlockResetGlobals(t)
	now := time.Now()
	markGrokModelQuotaBlock(5, "grok-4.5", now.Add(2*time.Hour))
	gatewayGrokFreeQuotaGateCache.Store(int64(5), grokFreeQuotaGateCacheEntry{tokens: 1, checkedAt: now, known: true})

	var svc *OpenAIGatewayService
	result := svc.ClearAccountRuntimeBlocks(context.Background(), nil)

	require.Equal(t, 1, result.Cleared.GrokModelQuotaBlocks)
	require.Equal(t, 1, result.Cleared.GrokFreeQuotaGates)
	require.Zero(t, result.Cleared.AccountRuntimeBlocks)
	require.False(t, isGrokModelQuotaBlocked(5, "grok-4.5", now))
}

// 并发安全：一边有请求路径不停往六类表里写封锁，一边有运维反复调逃生口。
// 每个 store 都有自己的锁，清空必须走锁，配合 -race 跑这个用例才有意义。
func TestClearAccountRuntimeBlocks_ConcurrentWithWriters(t *testing.T) {
	resetRuntimeBlockResetGlobals(t)
	svc := &OpenAIGatewayService{}
	transient := svc.getOpenAIAccountModelTransientState()
	circuit := svc.getOpenAIProxyStreamCircuit()
	grok := newGrokRuntimeBlockResetAccount(90, "team-concurrent")

	const writerCount = 6
	const writerRounds = 200
	const clearerCount = 3
	const clearerRounds = 20

	var writers sync.WaitGroup
	for w := 0; w < writerCount; w++ {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			account := newRuntimeBlockResetAccount(int64(100 + w))
			proxyID := int64(200 + w)
			for i := 0; i < writerRounds; i++ {
				now := time.Now()
				svc.BlockAccountScheduling(account, now.Add(time.Minute), "concurrent")
				transient.recordFailure(account.ID, "gpt-5", now)
				circuit.recordFailure(proxyID, now)
				markGrokModelQuotaBlock(account.ID, "grok-4.5", now.Add(time.Hour))
				markGrokTeamModelRateLimit(grok, "grok-4.5", now.Add(time.Hour))
				gatewayGrokFreeQuotaGateCache.Store(account.ID, grokFreeQuotaGateCacheEntry{checkedAt: now, known: true})
				openaiGrokFreeQuotaGateCache.Store(account.ID, grokFreeQuotaGateCacheEntry{checkedAt: now, known: true})
			}
		}(w)
	}

	var clearers sync.WaitGroup
	for c := 0; c < clearerCount; c++ {
		clearers.Add(1)
		go func() {
			defer clearers.Done()
			for i := 0; i < clearerRounds; i++ {
				svc.ClearAccountRuntimeBlocks(context.Background(), nil)
				svc.ClearAccountRuntimeBlocks(context.Background(), []int64{101, 102, 103})
			}
		}()
	}

	writers.Wait()
	clearers.Wait()

	// 写入停了之后再清一次，六类状态必须彻底空掉
	svc.ClearAccountRuntimeBlocks(context.Background(), nil)

	require.Zero(t, transient.size())
	require.Zero(t, circuit.activeBlockCount(time.Now()))
	for w := 0; w < writerCount; w++ {
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(newRuntimeBlockResetAccount(int64(100+w))))
	}
	globalGrokModelQuotaBlocks.mu.Lock()
	require.Empty(t, globalGrokModelQuotaBlocks.items)
	globalGrokModelQuotaBlocks.mu.Unlock()
	globalGrokTeamModelRateLimits.mu.Lock()
	require.Empty(t, globalGrokTeamModelRateLimits.items)
	globalGrokTeamModelRateLimits.mu.Unlock()
	require.Zero(t, clearGrokFreeQuotaGateCache(&gatewayGrokFreeQuotaGateCache, nil))
	require.Zero(t, clearGrokFreeQuotaGateCache(&openaiGrokFreeQuotaGateCache, nil))
}
