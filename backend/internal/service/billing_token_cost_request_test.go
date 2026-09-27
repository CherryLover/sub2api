//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newTokenCostTestEnv(t *testing.T, groupPlatform string, pricing []ChannelModelPricing, catalog *PricingService) (*BillingService, *ModelPricingResolver) {
	t.Helper()
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{{
				ID: 1, Name: "ch", Status: StatusActive, GroupIDs: []int64{100}, ModelPricing: pricing,
			}}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{100: groupPlatform}, nil
		},
	}
	cs := NewChannelService(repo, nil, nil, nil)
	bs := NewBillingService(&config.Config{}, catalog)
	return bs, NewModelPricingResolver(cs, bs)
}

func TestCalculateTokenCostForRequest_NoResolverFallsBackToCatalog(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 10}
	got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{Model: "gpt-5.4", Tokens: tokens, RateMultiplier: 1})
	require.NoError(t, err)
	want, err := bs.CalculateCost("gpt-5.4", tokens, 1)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestCalculateTokenCostForRequest_ChannelReasoningMultiplier(t *testing.T) {
	bs, resolver := newTokenCostTestEnv(t, PlatformAnthropic, []ChannelModelPricing{{
		Platform: PlatformAnthropic, Models: []string{"claude-opus-5-5"}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(10e-6), OutputPrice: testPtrFloat64(50e-6),
		ReasoningEffortMultipliers: map[string]float64{"high": 1.5, "max": 3},
	}}, nil)
	group := &Group{ID: 100, Platform: PlatformAnthropic}
	groupID := group.ID
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-opus-5-5", GroupID: &groupID, Group: group})

	base, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
		Ctx: context.Background(), Model: "claude-opus-5-5", Group: group,
		Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 100}, RateMultiplier: 0.8,
		Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)

	for effort, multiplier := range map[string]float64{"high": 1.5, "max": 3, "low": 1} {
		got, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
			Ctx: context.Background(), Model: "claude-opus-5-5", Group: group,
			Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 100}, RateMultiplier: 0.8,
			ReasoningEffort: effort, Resolver: resolver, Resolved: resolved,
		})
		require.NoError(t, err)
		require.InDelta(t, base.TotalCost*multiplier, got.TotalCost, 1e-12, effort)
		require.InDelta(t, base.ActualCost*multiplier, got.ActualCost, 1e-12, effort)
	}
}

func TestCalculateTokenCostForRequest_ReasoningMultiplierStacksWithServiceTier(t *testing.T) {
	fast := 2.5
	bs, resolver := newTokenCostTestEnv(t, PlatformOpenAI, []ChannelModelPricing{{
		Platform: PlatformOpenAI, Models: []string{"custom-model"}, BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(2e-6),
		FastMultiplier: &fast, ReasoningEffortMultipliers: map[string]float64{"high": 1.7},
	}}, nil)
	group := &Group{ID: 100, Platform: PlatformOpenAI}
	groupID := group.ID
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "custom-model", GroupID: &groupID, Group: group})

	standard, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
		Ctx: context.Background(), Model: "custom-model", Group: group,
		Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 100}, RateMultiplier: 1,
		ServiceTier: "fast", Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	adjusted, err := bs.CalculateTokenCostForRequest(TokenCostRequest{
		Ctx: context.Background(), Model: "custom-model", Group: group,
		Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 100}, RateMultiplier: 1,
		ServiceTier: "fast", ReasoningEffort: "high", Resolver: resolver, Resolved: resolved,
	})
	require.NoError(t, err)
	require.InDelta(t, standard.TotalCost*1.7, adjusted.TotalCost, 1e-12)
	require.InDelta(t, standard.ActualCost*1.7, adjusted.ActualCost, 1e-12)
}
