//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiCacheQuoteDoesNotTrustStaleLimitFlags(t *testing.T) {
	gateway, input := newGeminiCacheQuoteTest(t)
	input.APIKey.RateLimit5h = 0
	input.APIKey.RateLimit1d = 0
	input.APIKey.RateLimit7d = 0
	quote, err := gateway.QuoteGeminiCachedContent(context.Background(), input, 1000)
	require.NoError(t, err)
	require.True(t, quote.EnforceLimits)
	require.InDelta(t, quote.Log.ActualCost, quote.Command.APIKeyRateLimitCost, 1e-8, "SQL must be able to apply limits enabled while the request was in flight")
	gateway.cfg.RunMode = config.RunModeSimple
	gateway.cfg.SimpleModeKeyRateLimitEnabled = true
	quote, err = gateway.QuoteGeminiCachedContent(context.Background(), input, 1000)
	require.NoError(t, err)
	require.True(t, quote.EnforceLimits)
	require.Zero(t, quote.Command.APIKeyQuotaCost)
	require.Positive(t, quote.Command.APIKeyRateLimitCost)
	gateway.cfg.SimpleModeKeyRateLimitEnabled = false
	quote, err = gateway.QuoteGeminiCachedContent(context.Background(), input, 1000)
	require.NoError(t, err)
	require.False(t, quote.EnforceLimits)
	require.Zero(t, quote.Command.APIKeyRateLimitCost)
}
