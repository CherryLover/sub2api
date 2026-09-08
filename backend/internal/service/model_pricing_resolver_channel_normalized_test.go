package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupChannelPricingNormalizedFallsBackToBaseModel(t *testing.T) {
	const groupID int64 = 777
	inputPerMillion := 0.4
	pricing := ChannelModelPricing{
		Platform:    PlatformOpenAI,
		Models:      []string{"gpt-5.6-luna"},
		BillingMode: BillingModeToken,
		InputPrice:  float64Ptr(inputPerMillion / 1e6),
	}
	cs := &ChannelService{}
	cs.cache.Store(populateChannelCache([]Channel{{
		ID:           1,
		Name:         "codex-channel",
		Status:       StatusActive,
		ModelPricing: []ChannelModelPricing{pricing},
		GroupIDs:     []int64{groupID},
	}}, map[int64]string{groupID: PlatformOpenAI}))

	resolver := &ModelPricingResolver{channelService: cs}
	got := resolver.lookupChannelPricingNormalized(context.Background(), groupID, "gpt-5.6-luna-high")
	require.NotNil(t, got)
	require.NotNil(t, got.InputPrice)
	require.InDelta(t, inputPerMillion/1e6, *got.InputPrice, 1e-12)
}

func TestLookupChannelPricingNormalizedPrefersLiteralVariant(t *testing.T) {
	const groupID int64 = 778
	base := ChannelModelPricing{
		Platform:    PlatformOpenAI,
		Models:      []string{"gpt-5.6-luna"},
		BillingMode: BillingModeToken,
		InputPrice:  float64Ptr(0.4 / 1e6),
	}
	variant := ChannelModelPricing{
		Platform:    PlatformOpenAI,
		Models:      []string{"gpt-5.6-luna-high"},
		BillingMode: BillingModeToken,
		InputPrice:  float64Ptr(0.9 / 1e6),
	}
	cs := &ChannelService{}
	cs.cache.Store(populateChannelCache([]Channel{{
		ID:           2,
		Name:         "codex-channel",
		Status:       StatusActive,
		ModelPricing: []ChannelModelPricing{base, variant},
		GroupIDs:     []int64{groupID},
	}}, map[int64]string{groupID: PlatformOpenAI}))

	resolver := &ModelPricingResolver{channelService: cs}
	got := resolver.lookupChannelPricingNormalized(context.Background(), groupID, "gpt-5.6-luna-high")
	require.NotNil(t, got)
	require.NotNil(t, got.InputPrice)
	require.InDelta(t, 0.9/1e6, *got.InputPrice, 1e-12)
}
