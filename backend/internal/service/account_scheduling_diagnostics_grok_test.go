//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newGrokDiagnosisAccount(t *testing.T, id int64, teamID string) *Account {
	t.Helper()
	return &Account{
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"team_id": teamID},
	}
}

// 这两个 Grok 专属的进程内封锁此前任何页面都看不到：账号被网关跳过，
// 列表里却是绿色的「正常」。team 那条尤其隐蔽——本账号一次都没失败过，
// 是同 team 的兄弟账号撞了 429 把它连坐了。
func TestDiagnoseAccountScheduling_ReportsGrokModelScopedBlocks(t *testing.T) {
	svc := &OpenAIGatewayService{}
	now := time.Now()
	account := newGrokDiagnosisAccount(t, 30, "team-alpha")

	markGrokModelQuotaBlock(account.ID, "grok-4", now.Add(2*time.Hour))
	markGrokTeamModelRateLimit(account, "grok-4-fast", now.Add(20*time.Minute))
	t.Cleanup(func() {
		clearGrokModelScopedBlocksForTest(account)
	})

	d := svc.DiagnoseAccountScheduling(context.Background(), []*Account{account})[30]

	require.True(t, d.Schedulable, "两者都是模型级，不该让整号变成不可调度")
	require.Equal(t, []string{
		AccountSchedulingBlockGrokModelQuota,
		AccountSchedulingBlockGrokTeamRateLimit,
	}, schedulingDiagnosisSources(d))

	require.Equal(t, "grok-4", d.Blocks[0].Model)
	require.NotNil(t, d.Blocks[0].Until)
	require.True(t, d.Blocks[0].Until.After(now))

	require.Equal(t, "grok-4-fast", d.Blocks[1].Model)
	require.NotNil(t, d.Blocks[1].Until)
	require.True(t, d.Blocks[1].Until.After(now))
}

// 诊断只读：不得像调度路径那样顺手清理过期项。
func TestDiagnoseAccountScheduling_GrokPeeksDoNotMutateOrLeakAcrossAccounts(t *testing.T) {
	svc := &OpenAIGatewayService{}
	now := time.Now()
	account := newGrokDiagnosisAccount(t, 31, "team-beta")
	otherTeam := newGrokDiagnosisAccount(t, 32, "team-gamma")

	markGrokModelQuotaBlock(account.ID, "grok-4", now.Add(2*time.Hour))
	markGrokTeamModelRateLimit(account, "grok-4", now.Add(20*time.Minute))
	// 手工塞一条已经过期的，诊断既不能报它、也不能删它。
	globalGrokModelQuotaBlocks.mu.Lock()
	globalGrokModelQuotaBlocks.items[grokModelQuotaBlockKey(account.ID, "grok-stale")] =
		grokModelQuotaBlock{Until: now.Add(-time.Minute)}
	before := len(globalGrokModelQuotaBlocks.items)
	globalGrokModelQuotaBlocks.mu.Unlock()

	t.Cleanup(func() {
		clearGrokModelScopedBlocksForTest(account)
		globalGrokModelQuotaBlocks.mu.Lock()
		delete(globalGrokModelQuotaBlocks.items, grokModelQuotaBlockKey(account.ID, "grok-stale"))
		globalGrokModelQuotaBlocks.mu.Unlock()
	})

	result := svc.DiagnoseAccountScheduling(context.Background(), []*Account{account, otherTeam})

	models := make([]string, 0, 2)
	for _, block := range result[31].Blocks {
		models = append(models, block.Model)
	}
	require.Equal(t, []string{"grok-4", "grok-4"}, models, "过期那条不该出现")

	globalGrokModelQuotaBlocks.mu.Lock()
	after := len(globalGrokModelQuotaBlocks.items)
	globalGrokModelQuotaBlocks.mu.Unlock()
	require.Equal(t, before, after, "诊断不得删除过期项")

	require.Empty(t, result[32].Blocks, "另一个 team 的账号不该被连坐")
	require.True(t, result[32].Schedulable)
}

// 非 Grok 平台不查这两张表。
func TestDiagnoseAccountScheduling_SkipsGrokBlocksForNonGrokAccounts(t *testing.T) {
	svc := &OpenAIGatewayService{}
	openai := newSchedulingDiagnosisAccount(33, AccountTypeOAuth)
	markGrokModelQuotaBlock(openai.ID, "grok-4", time.Now().Add(2*time.Hour))
	t.Cleanup(func() {
		globalGrokModelQuotaBlocks.mu.Lock()
		delete(globalGrokModelQuotaBlocks.items, grokModelQuotaBlockKey(openai.ID, "grok-4"))
		globalGrokModelQuotaBlocks.mu.Unlock()
	})

	d := svc.DiagnoseAccountScheduling(context.Background(), []*Account{openai})[33]
	require.Empty(t, d.Blocks)
	require.True(t, d.Schedulable)
}

func clearGrokModelScopedBlocksForTest(account *Account) {
	globalGrokModelQuotaBlocks.mu.Lock()
	for key := range globalGrokModelQuotaBlocks.items {
		delete(globalGrokModelQuotaBlocks.items, key)
	}
	globalGrokModelQuotaBlocks.mu.Unlock()

	fp := grokTeamFingerprint(accountGrokTeamID(account))
	globalGrokTeamModelRateLimits.mu.Lock()
	for key := range globalGrokTeamModelRateLimits.items {
		if fp == "" || len(key) >= len(fp) && key[:len(fp)] == fp {
			delete(globalGrokTeamModelRateLimits.items, key)
		}
	}
	globalGrokTeamModelRateLimits.mu.Unlock()
}
