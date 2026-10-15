//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type geminiCacheQuotaUpdateStub struct{}

func (*geminiCacheQuotaUpdateStub) UpdateQuotaUsed(context.Context, int64, float64) error { return nil }
func (*geminiCacheQuotaUpdateStub) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func newGeminiCacheQuoteTest(t *testing.T) (*GatewayService, *RecordUsageInput) {
	t.Helper()
	p := &PricingService{}
	data, err := p.parsePricingData([]byte(`{"gemini-2.5-pro":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000008,"cache_storage_cost_per_token_per_hour":0.000004,"litellm_provider":"gemini"}}`))
	require.NoError(t, err)
	p.pricingData = data
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	bill := NewBillingService(cfg, p)
	gateway := &GatewayService{cfg: cfg, billingService: bill}
	group := &Group{ID: 10, Platform: PlatformGemini, RateMultiplier: 2, Hydrated: true}
	user := &User{ID: 11}
	key := &APIKey{ID: 12, UserID: user.ID, User: user, GroupID: &group.ID, Group: group, RateLimit5h: 100}
	account := &Account{ID: 13, Platform: PlatformGemini, Type: AccountTypeAPIKey}
	in := &RecordUsageInput{Result: &ForwardResult{RequestID: "gcache:create:abc", Model: "gemini-2.5-pro", UpstreamModel: "gemini-2.5-pro", Usage: ClaudeUsage{InputTokens: 1000}}, APIKey: key, User: user, Account: account, APIKeyService: &geminiCacheQuotaUpdateStub{}, PricingAt: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}
	return gateway, in
}

func TestGeminiCacheSettlementQuoteCreateAndRenew(t *testing.T) {
	s, in := newGeminiCacheQuoteTest(t)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "reused-client-id")
	q, err := s.QuoteGeminiCachedContent(ctx, in, 2000)
	require.NoError(t, err)
	require.InDelta(t, 0.002, q.Log.InputCost, 1e-12)
	require.InDelta(t, 0.010, q.Log.TotalCost, 1e-12)
	require.InDelta(t, 0.020, q.Log.ActualCost, 1e-12)
	require.InDelta(t, 0.020, q.Command.APIKeyQuotaCost, 1e-12)
	require.InDelta(t, 0.020, q.Command.APIKeyRateLimitCost, 1e-12)
	require.Equal(t, "gcache:create:abc", q.Log.RequestID)
	require.True(t, q.EnforceLimits)
	in.Result.RequestID = "gcache:patch:abc:nonce"
	in.Result.Usage.InputTokens = 0
	perRequest := 10.0
	in.APIKey.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"gemini-2.5-pro"}, BillingMode: BillingModePerRequest, PerRequestPrice: &perRequest}}
	s.resolver = NewModelPricingResolver(nil, s.billingService)
	q, err = s.QuoteGeminiCachedContent(ctx, in, 1000)
	require.NoError(t, err)
	require.Zero(t, q.Log.InputTokens)
	require.Zero(t, q.Log.InputCost)
	require.InDelta(t, 0.004, q.Log.TotalCost, 1e-12, "renewal is storage-only, not a $10 request")
	require.InDelta(t, 0.008, q.Command.APIKeyQuotaCost, 1e-12)
	q, err = s.QuoteGeminiCachedContent(ctx, in, 0)
	require.NoError(t, err)
	require.Zero(t, q.Log.ActualCost, "shortening TTL cannot debit or refund previously paid storage")
}

func TestGeminiCacheSettlementQuoteRejectsUnsafeInputs(t *testing.T) {
	for _, amount := range []float64{-1, math.NaN(), math.Inf(1)} {
		s, in := newGeminiCacheQuoteTest(t)
		_, err := s.QuoteGeminiCachedContent(context.Background(), in, amount)
		require.Error(t, err)
	}
	s, in := newGeminiCacheQuoteTest(t)
	in.APIKeyService = nil
	_, err := s.QuoteGeminiCachedContent(context.Background(), in, 1)
	require.ErrorIs(t, err, ErrGeminiCacheBillingUnavailable)
	s, in = newGeminiCacheQuoteTest(t)
	in.Result.Model = "unpriced"
	in.Result.UpstreamModel = "unpriced"
	_, err = s.QuoteGeminiCachedContent(context.Background(), in, 1)
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
	s, in = newGeminiCacheQuoteTest(t)
	zero := 0.0
	s.billingService.pricingService.pricingData["gemini-2.5-pro"].CacheStorageCostPerTokenPerHour = &zero
	_, err = s.QuoteGeminiCachedContent(context.Background(), in, 1)
	require.ErrorIs(t, err, ErrModelPricingUnavailable, "explicit invalid storage price cannot silently enable free storage")
}

func TestGeminiCacheSettlementSimpleMode(t *testing.T) {
	s, in := newGeminiCacheQuoteTest(t)
	s.cfg.RunMode = config.RunModeSimple
	q, err := s.QuoteGeminiCachedContent(context.Background(), in, 2000)
	require.NoError(t, err)
	require.Zero(t, q.Command.APIKeyQuotaCost)
	require.Zero(t, q.Command.APIKeyRateLimitCost)
	require.Zero(t, q.Command.AccountQuotaCost)
	require.False(t, q.EnforceLimits)
	s.cfg.SimpleModeKeyRateLimitEnabled = true
	q, err = s.QuoteGeminiCachedContent(context.Background(), in, 2000)
	require.NoError(t, err)
	require.Zero(t, q.Command.APIKeyQuotaCost)
	require.Positive(t, q.Command.APIKeyRateLimitCost)
	require.Zero(t, q.Command.AccountQuotaCost)
	require.True(t, q.EnforceLimits)
}

func TestGeminiCacheSettlementOwnership(t *testing.T) {
	s, in := newGeminiCacheQuoteTest(t)
	q, err := s.QuoteGeminiCachedContent(context.Background(), in, 1000)
	require.NoError(t, err)
	r := &GeminiCachedContent{APIKeyID: in.APIKey.ID, UserID: in.User.ID, GroupID: *in.APIKey.GroupID, AccountID: in.Account.ID}
	require.NoError(t, ValidateGeminiCachedContentSettlement(r, q))
	r.APIKeyID++
	require.Error(t, ValidateGeminiCachedContentSettlement(r, q))
}
