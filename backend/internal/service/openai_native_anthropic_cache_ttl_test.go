//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpenAINativeAnthropicCacheTTLUsageIsExclusive(t *testing.T) {
	for _, tc := range []struct {
		usage                         OpenAIUsage
		input, creation, int5m, int1h int
	}{
		{OpenAIUsage{InputTokens: 100, CacheReadInputTokens: 20, CacheCreationInputTokens: 40, CacheCreation5mTokens: 10, CacheCreation1hTokens: 30}, 40, 40, 10, 30},
		{OpenAIUsage{InputTokens: 100, CacheReadInputTokens: 20, CacheCreation5mTokens: 10, CacheCreation1hTokens: 30}, 40, 40, 10, 30},
		{OpenAIUsage{InputTokens: 100, CacheReadInputTokens: 20, CacheCreationInputTokens: 40, CacheCreation5mTokens: 100, CacheCreation1hTokens: 200}, 40, 40, 100, 200},
	} {
		tokens := tc.usage.billingUsageTokens()
		require.Equal(t, tc.input, tokens.InputTokens)
		require.Equal(t, tc.creation, tokens.CacheCreationTokens)
		require.Equal(t, tc.int5m, tokens.CacheCreation5mTokens)
		require.Equal(t, tc.int1h, tokens.CacheCreation1hTokens)
		five, one := normalizeCacheCreationBreakdown(tokens)
		require.Equal(t, tc.creation, five+one)
	}
}
