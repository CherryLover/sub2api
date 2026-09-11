//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 新版 OpenAI 调度器以前完全不读 accounts.extra.model_rate_limits：
// 模型级限流写了没人读，被限流的 (账号, 模型) 组合照样被选中去撞 429。
// 这组用例锁住候选过滤 / 终检 / 粘性会话三处的模型级过滤。

type dedicatedQuotaPoolSchedulerRepo struct {
	AccountRepository
	accounts []Account
}

func (r *dedicatedQuotaPoolSchedulerRepo) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r *dedicatedQuotaPoolSchedulerRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
		}
	}
	return nil, errors.New("account not found")
}

// dedicatedQuotaPoolStickyCache 只需要非 nil：模型级限流命中时必须在任何缓存写操作
// 之前早退，所以 DeleteSessionAccountID 被调用即代表绑定被误删。
type dedicatedQuotaPoolStickyCache struct {
	GatewayCache
	deleteCalls int
}

func (c *dedicatedQuotaPoolStickyCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, ErrStickySessionNotFound
}

func (c *dedicatedQuotaPoolStickyCache) DeleteSessionAccountID(context.Context, int64, string) error {
	c.deleteCalls++
	return nil
}

func sparkPoolLimitedAccount(id int64) Account {
	account := Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 5,
	}
	setAccountModelRateLimitSnapshot(&account, "gpt-5.3-codex-spark",
		time.Now().Add(4*24*time.Hour), openAIDedicatedQuotaPoolRateLimitReason, time.Now())
	return account
}

func healthyOpenAIAccount(id int64) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 5,
	}
}

func dedicatedQuotaPoolSchedulerConfig() *config.Config {
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	return cfg
}

// 候选过滤：被限流的那个模型跳过该账号，同账号的其他模型仍然照常选中。
func TestOpenAIScheduler_LoadBalanceSkipsModelRateLimitedAccount(t *testing.T) {
	accounts := []Account{sparkPoolLimitedAccount(7)}
	svc := &OpenAIGatewayService{
		cfg:         dedicatedQuotaPoolSchedulerConfig(),
		accountRepo: &dedicatedQuotaPoolSchedulerRepo{accounts: accounts},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	_, _, _, _, err := scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "gpt-5.3-codex-spark",
	})
	require.Error(t, err, "spark 池被限流的账号不得再被选去跑 spark")
	require.Contains(t, err.Error(), "model_rate_limited",
		"具名 reason 必须出现在过滤统计里，便于排查『无可用账号』")

	// 同一个账号，主池模型仍然完全可用。
	for _, model := range []string{"gpt-6-astra", "gpt-5.5", "gpt-5.6-sol"} {
		selection, _, _, _, err := scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{
			Platform:       PlatformOpenAI,
			RequestedModel: model,
		})
		require.NoError(t, err, "主池模型 %s 不应被 spark 池的限流牵连", model)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		require.Equal(t, int64(7), selection.Account.ID)
	}
}

// 号池里还有健康账号时，spark 请求应该落到健康账号上，而不是报『无可用账号』。
func TestOpenAIScheduler_LoadBalancePrefersAccountWithFreeSparkPool(t *testing.T) {
	accounts := []Account{sparkPoolLimitedAccount(7), healthyOpenAIAccount(8)}
	svc := &OpenAIGatewayService{
		cfg:         dedicatedQuotaPoolSchedulerConfig(),
		accountRepo: &dedicatedQuotaPoolSchedulerRepo{accounts: accounts},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	for range 5 {
		selection, _, _, _, err := scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{
			Platform:       PlatformOpenAI,
			RequestedModel: "gpt-5.3-codex-spark",
		})
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		require.Equal(t, int64(8), selection.Account.ID)
	}
}

// 终检点：isAccountRequestCompatibleReason 必须给出具名 reason。
// fresh / DB 复检与粘性路径都走这里，漏掉会让模型级限流在复检后被重新放行。
func TestOpenAIScheduler_IsAccountRequestCompatibleReasonModelRateLimited(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: dedicatedQuotaPoolSchedulerConfig()}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	account := sparkPoolLimitedAccount(7)

	compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), &account,
		OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.3-codex-spark"})
	require.False(t, compatible)
	require.Equal(t, "model_rate_limited", reason)

	compatible, reason = scheduler.isAccountRequestCompatibleReason(context.Background(), &account,
		OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-6-astra"})
	require.True(t, compatible, "主池模型不应被 spark 池的限流挡掉")
	require.Empty(t, reason)
}

// 粘性会话：必须跳过被模型级限流的账号，否则会话会一直把请求钉在已经撞满的池上；
// 同时不能删除绑定 —— 绑定对该会话的其他模型仍然有效。
func TestOpenAIScheduler_StickySessionSkipsModelRateLimitedWithoutDroppingBinding(t *testing.T) {
	accounts := []Account{sparkPoolLimitedAccount(7)}
	cache := &dedicatedQuotaPoolStickyCache{}
	svc := &OpenAIGatewayService{
		cfg:         dedicatedQuotaPoolSchedulerConfig(),
		accountRepo: &dedicatedQuotaPoolSchedulerRepo{accounts: accounts},
		cache:       cache,
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	selection, escaped, err := scheduler.selectBySessionHash(context.Background(), OpenAIAccountScheduleRequest{
		Platform:        PlatformOpenAI,
		RequestedModel:  "gpt-5.3-codex-spark",
		SessionHash:     "session-hash-7",
		StickyAccountID: 7,
	})

	require.NoError(t, err)
	require.Nil(t, selection, "粘性会话不得绕过模型级限流")
	require.False(t, escaped)
	require.Zero(t, cache.deleteCalls,
		"模型级限流只影响一个模型，不应打散整个会话的账号亲和性")
}
