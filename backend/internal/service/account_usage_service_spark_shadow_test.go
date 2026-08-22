package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sparkShadowUsageTestRepo is a minimal AccountRepository stub for spark shadow
// usage tests.  GetByID serves both shadow and parent accounts from a map;
// UpdateExtra records the persisted updates for assertion.
type sparkShadowUsageTestRepo struct {
	AccountRepository
	accounts      map[int64]*Account
	updateExtraCh chan map[string]any
}

func (r *sparkShadowUsageTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if acc, ok := r.accounts[id]; ok {
		return acc, nil
	}
	return nil, fmt.Errorf("account %d not found", id)
}

func (r *sparkShadowUsageTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

// TestGetOpenAIUsage_SparkShadow_WritesExtraAndReturnsNonEmptyWindows covers
// two assertions required by Task 3.2:
//
// A) After getOpenAIUsage on a spark shadow account the shadow row's
// Extra["codex_5h_used_percent"] is persisted, and the upstream call carried
// the PARENT account's chatgpt-account-id (not the shadow's empty one).
//
// B) (P1-b regression guard) The UsageInfo RETURNED by the same call has
// non-nil FiveHour AND SevenDay windows — proving that the rebuild happened
// and not just the DB write.
func TestGetOpenAIUsage_SparkShadow_WritesExtraAndReturnsNonEmptyWindows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pid := int64(100)
	shadow := &Account{
		ID:              200,
		ParentAccountID: &pid,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		QuotaDimension:  QuotaDimensionSpark,
	}
	parent := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-spark-parent",
		},
	}

	// Repo shared by both the OpenAIQuotaService (needs shadow+parent for resolve)
	// and the AccountUsageService (needs UpdateExtra for persist).
	updateExtraCh := make(chan map[string]any, 1)
	repo := &sparkShadowUsageTestRepo{
		accounts:      map[int64]*Account{200: shadow, 100: parent},
		updateExtraCh: updateExtraCh,
	}

	// Token cache: return a fake token for the parent account key.
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(parent): "fake-access-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	// httptest server: records the chatgpt-account-id header and returns a
	// synthetic OpenAIQuotaUsage with codex_bengalfox 5h+7d windows.
	var capturedAccountID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAccountID = r.Header.Get("chatgpt-account-id")
		w.Header().Set("content-type", "application/json")
		resp := OpenAIQuotaUsage{
			AdditionalRateLimits: []OpenAIAdditionalRateLimit{
				{
					MeteredFeature: "codex_bengalfox",
					RateLimit: &OpenAIRateLimit{
						// Primary window → 5h (18000 s = 300 min)
						PrimaryWindow: &OpenAIRateLimitWindow{
							UsedPercent:        42.5,
							ResetAfterSeconds:  3600,
							LimitWindowSeconds: 18000,
						},
						// Secondary window → 7d (604800 s = 10080 min)
						SecondaryWindow: &OpenAIRateLimitWindow{
							UsedPercent:        10.0,
							ResetAfterSeconds:  86400,
							LimitWindowSeconds: 604800,
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	quotaService := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	svc := &AccountUsageService{
		accountRepo:        repo,
		openAIQuotaService: quotaService,
	}

	usage, err := svc.getOpenAIUsage(ctx, shadow, true /*force*/)
	require.NoError(t, err)

	// Assertion A-1: upstream received the PARENT's chatgpt-account-id.
	require.Equal(t, "org-spark-parent", capturedAccountID,
		"QueryUsage must use parent's chatgpt-account-id for spark shadow accounts")

	// Assertion A-2: shadow Extra was persisted with codex_5h_used_percent.
	select {
	case updates := <-updateExtraCh:
		require.Contains(t, updates, "codex_5h_used_percent",
			"persisted extra must contain codex_5h_used_percent")
		require.InDelta(t, 42.5, updates["codex_5h_used_percent"], 0.01,
			"codex_5h_used_percent must match the upstream value")
	case <-time.After(2 * time.Second):
		t.Fatal("UpdateExtra was not called within timeout — spark shadow persist did not happen")
	}

	// Assertion B (P1-b regression guard): returned UsageInfo must have
	// non-nil windows. This FAILS if the code only writes Extra without
	// rebuilding the returned UsageInfo.
	require.NotNil(t, usage.FiveHour,
		"returned UsageInfo.FiveHour must be non-nil (rebuild from merged Extra must happen)")
	require.NotNil(t, usage.SevenDay,
		"returned UsageInfo.SevenDay must be non-nil (rebuild from merged Extra must happen)")
}

// sparkShadowSelfHealRepo 在 sparkShadowUsageTestRepo 之上记录任何"清限流"动作。
// 它实现两个清除原语，这样测试断言的是"守卫生效"，而不是"stub 恰好没有这个方法"。
type sparkShadowSelfHealRepo struct {
	sparkShadowUsageTestRepo
	unconditionalClears []int64
	observedClears      []int64
}

func (r *sparkShadowSelfHealRepo) ClearRateLimit(_ context.Context, id int64) error {
	r.unconditionalClears = append(r.unconditionalClears, id)
	return nil
}

func (r *sparkShadowSelfHealRepo) ClearOpenAIRateLimitIfObserved(_ context.Context, id int64, _, _ time.Time) (bool, error) {
	r.observedClears = append(r.observedClears, id)
	return true, nil
}

// TestGetOpenAIUsage_SparkShadowRateLimitedIsNeverSelfHealed 走真实入口 getOpenAIUsage，
// 而不是直接调私有的自愈函数。
//
// 私有函数里那句 `if account.IsShadow() { return }` 在真实路径上是够不到的死代码：影子在
// getOpenAIUsage 里走的是 `if account.IsShadow()` 的 QueryUsage 分支，根本不进 else。
// 真正需要锁住的性质是"影子不会经由 else 分支泄漏进自愈"——只有从 getOpenAIUsage 进去才测得到。
//
// 场景：一个被限流的 spark 影子，上游 /wham/usage 返回两个窗口都很健康的 codex_bengalfox 快照。
// 期望：限流状态原封不动（影子的限流只由 bengalfox 道自己维护），没有任何清除调用。
func TestGetOpenAIUsage_SparkShadowRateLimitedIsNeverSelfHealed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pid := int64(300)
	limitedAt := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)
	resetAt := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Second)
	shadow := &Account{
		ID:               400,
		ParentAccountID:  &pid,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		Status:           StatusActive,
		QuotaDimension:   QuotaDimensionSpark,
		RateLimitedAt:    &limitedAt,
		RateLimitResetAt: &resetAt,
	}
	parent := &Account{
		ID:       300,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-spark-parent",
		},
	}

	updateExtraCh := make(chan map[string]any, 4)
	repo := &sparkShadowSelfHealRepo{
		sparkShadowUsageTestRepo: sparkShadowUsageTestRepo{
			accounts:      map[int64]*Account{400: shadow, 300: parent},
			updateExtraCh: updateExtraCh,
		},
	}

	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(parent): "fake-access-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		resp := OpenAIQuotaUsage{
			AdditionalRateLimits: []OpenAIAdditionalRateLimit{
				{
					MeteredFeature: "codex_bengalfox",
					RateLimit: &OpenAIRateLimit{
						PrimaryWindow: &OpenAIRateLimitWindow{
							UsedPercent:        4.0,
							ResetAfterSeconds:  3600,
							LimitWindowSeconds: 18000,
						},
						SecondaryWindow: &OpenAIRateLimitWindow{
							UsedPercent:        7.0,
							ResetAfterSeconds:  86400,
							LimitWindowSeconds: 604800,
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	quotaService := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	svc := &AccountUsageService{
		accountRepo:        repo,
		openAIQuotaService: quotaService,
	}

	usage, err := svc.getOpenAIUsage(ctx, shadow, true /*force*/)
	require.NoError(t, err)

	// 快照确实是"健康"的——否则这个用例什么都没证明。
	require.NotNil(t, usage.FiveHour)
	require.NotNil(t, usage.SevenDay)
	require.Less(t, usage.FiveHour.Utilization, 100.0)
	require.Less(t, usage.SevenDay.Utilization, 100.0)

	require.Empty(t, repo.unconditionalClears,
		"spark shadow must never be unconditionally cleared by a global codex self-heal")
	require.Empty(t, repo.observedClears,
		"spark shadow must never reach the OpenAI 429 self-heal at all")
	require.True(t, shadow.IsRateLimited(),
		"spark shadow rate limit is owned by the bengalfox channel and must survive a usage refresh")
}
