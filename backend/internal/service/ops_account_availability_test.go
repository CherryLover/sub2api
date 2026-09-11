//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// opsAvailabilityAccountRepoStub 只实现运维页取账号那一个方法，其余靠嵌入接口占位。
type opsAvailabilityAccountRepoStub struct {
	AccountRepository
	accounts []Account
}

func (s *opsAvailabilityAccountRepoStub) ListOpsAccountsForStats(
	_ context.Context, _ string, _ *int64,
) ([]Account, error) {
	return s.accounts, nil
}

// 运维页的"可用率"曾经自己手写一套 status + schedulable + 限流 + 过载 + 临时停调 的判定，
// 于是过期、额度耗尽、配额自动暂停和全部进程内停用统统被算成"可用"——公司那次事故里
// 账号被网关封死，这张卡片显示的却是 100% 可用。现在它必须与调度诊断同源。
func TestGetAccountAvailabilityStats_UsesSchedulingDiagnosis(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	healthy := Account{
		ID: 1, Name: "healthy", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
	}
	// 旧口径会把它算成"可用"：status=active、schedulable=true、三个窗口都没命中。
	expired := Account{
		ID: 2, Name: "expired", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		AutoPauseOnExpired: true, ExpiresAt: &past,
	}
	rateLimited := Account{
		ID: 3, Name: "rate-limited", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, RateLimitResetAt: &future,
	}

	svc := &OpsService{accountRepo: &opsAvailabilityAccountRepoStub{
		accounts: []Account{healthy, expired, rateLimited},
	}}

	platform, _, accounts, _, err := svc.GetAccountAvailabilityStats(context.Background(), "", nil)
	require.NoError(t, err)

	require.True(t, accounts[1].IsAvailable)
	require.False(t, accounts[2].IsAvailable, "已过期且开了自动暂停的账号不可调度")
	require.False(t, accounts[3].IsAvailable)

	require.EqualValues(t, 3, platform[PlatformOpenAI].TotalAccounts)
	require.EqualValues(t, 1, platform[PlatformOpenAI].AvailableCount)

	// 原因清单要跟着一起返回，否则页面只知道"不可用"却说不出为什么。
	require.Equal(t, []string{AccountSchedulingBlockExpired}, schedulingDiagnosisSources(
		AccountSchedulingDiagnosis{Blocks: accounts[2].SchedulingBlocks},
	))
	require.Empty(t, accounts[1].SchedulingBlocks)
}

// 进程内停用是这次事故的核心：数据库里一切正常，网关就是不调度它。
func TestGetAccountAvailabilityStats_CountsInProcessRuntimeBlock(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	blocked := Account{
		ID: 7, Name: "runtime-blocked", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
	}
	gateway.BlockAccountScheduling(&blocked, time.Now().Add(time.Hour), "429")

	svc := &OpsService{
		accountRepo:          &opsAvailabilityAccountRepoStub{accounts: []Account{blocked}},
		openAIGatewayService: gateway,
	}

	_, _, accounts, _, err := svc.GetAccountAvailabilityStats(context.Background(), "", nil)
	require.NoError(t, err)

	require.False(t, accounts[7].IsAvailable, "数据库全绿但进程内被停用，不能算可用")
	require.Equal(t, []string{AccountSchedulingBlockRuntime}, schedulingDiagnosisSources(
		AccountSchedulingDiagnosis{Blocks: accounts[7].SchedulingBlocks},
	))
}

// 网关未注入时（例如精简装配）退回数据库派生的判定，不能整页报错。
func TestGetAccountAvailabilityStats_NilGatewayFallsBackToPersisted(t *testing.T) {
	future := time.Now().Add(time.Hour)
	overloaded := Account{
		ID: 9, Name: "overloaded", Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, OverloadUntil: &future,
	}

	svc := &OpsService{accountRepo: &opsAvailabilityAccountRepoStub{accounts: []Account{overloaded}}}

	_, _, accounts, _, err := svc.GetAccountAvailabilityStats(context.Background(), "", nil)
	require.NoError(t, err)
	require.False(t, accounts[9].IsAvailable)
}
