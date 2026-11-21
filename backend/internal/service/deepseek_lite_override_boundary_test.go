//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Lite exposes group/channel pricing, not upstream's override-file product.
func TestDeepseekLiteGroupExplicitPricesIncludingZero(t *testing.T) {
	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		for _, zero := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/zero=%t", model, zero), func(t *testing.T) {
				input, output, cache := 2e-6, 3e-6, 4e-7
				if zero {
					input, output, cache = 0, 0, 0
				}
				group := &Group{ID: 922, Platform: PlatformDeepseek, Status: StatusActive, ModelPricing: []ChannelModelPricing{{Models: []string{model}, BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output, CacheReadPrice: &cache}}}
				svc := newTestBillingService()
				resolver := NewModelPricingResolver(nil, svc)
				for _, hour := range []int{2, 12} {
					cost, err := svc.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: model, Group: group, Tokens: UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 2000}, RateMultiplier: 1.25, Resolver: resolver, PricingAt: time.Date(2026, 8, 24, hour, 0, 0, 0, time.UTC)})
					require.NoError(t, err)
					want := 1000*input + 500*output + 2000*cache
					require.InDelta(t, want, cost.TotalCost, 1e-12)
					require.InDelta(t, want*1.25, cost.ActualCost, 1e-12)
				}
			})
		}
	}
}
