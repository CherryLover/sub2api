//go:build unit

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGPT61TerraLiteIdentityAndEffort(t *testing.T) {
	require.Contains(t, openai.DefaultModelIDs(), "gpt-6.1-terra")
	for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
		model := "openai/GPT-6.1-Terra-" + effort
		require.True(t, openai.IsGPT61TerraModelSpelling(model))
		require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel(model))
		require.Equal(t, effort, normalizeOpenAIReasoningEffortForModel(effort, model))
		req := &apicompat.AnthropicRequest{Model: model}
		applyOpenAICompatModelNormalization(req)
		require.Equal(t, "gpt-6.1-terra", req.Model)
		converted, err := apicompat.AnthropicToResponses(req)
		require.NoError(t, err)
		require.Equal(t, effort, converted.Reasoning.Effort)
	}
	for _, bad := range []string{"gpt-6.1-terra-ultra", "gpt-6.1-terra-minimal", "gpt-6.1-sol"} {
		require.False(t, openai.IsGPT61TerraModelSpelling(bad))
	}
	require.True(t, openai.RequiresResponsesForToolCalls("gpt-6.1-terra"))
	require.False(t, openai.RequiresResponsesForToolCalls("gpt-5.6-sol"))
}

func TestGPT61TerraLiteFallbackPricing(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	price := billing.getFallbackPricing("gpt-6.1-terra")
	require.NotNil(t, price)
	require.InDelta(t, 1e-6, price.InputPricePerToken, 1e-12)
	require.InDelta(t, 5e-6, price.OutputPricePerToken, 1e-12)
	require.InDelta(t, 0.05e-6, price.CacheReadPricePerToken, 1e-12)
	require.Equal(t, 272000, price.LongContextInputThreshold)
}
