//go:build unit

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGPT62LiteCatalogPricingAndEffort(t *testing.T) {
	svc := newTestBillingService()
	for _, tt := range []struct {
		base                 string
		input, output, cache float64
		context              int64
	}{
		{"gpt-6.2-sol", 2e-6, 10e-6, 0.1e-6, 500000},
		{"gpt-6.2-astra", 8e-6, 40e-6, 0.4e-6, 272000},
	} {
		model := "openai/" + tt.base + "-high"
		require.Equal(t, tt.base, normalizeKnownOpenAICodexModel(model))
		require.Equal(t, tt.base, NormalizeOpenAICompatRequestedModel(model))
		require.True(t, shouldAutoInjectPromptCacheKeyForCompat(model))
		require.True(t, openai.RequiresResponsesForToolCalls(model))
		require.NoError(t, validateGPT61CompatRequest([]byte(`{"reasoning":{"effort":"high"}}`), tt.base))
		require.Error(t, validateGPT61CompatRequest([]byte(`{"reasoning":{"effort":"minimal"}}`), tt.base))
		p, err := svc.GetModelPricing(tt.base)
		require.NoError(t, err)
		require.InDelta(t, tt.input, p.InputPricePerToken, 1e-12)
		require.InDelta(t, tt.output, p.OutputPricePerToken, 1e-12)
		require.InDelta(t, tt.cache, p.CacheReadPricePerToken, 1e-12)
		descriptor := newConfiguredCodexModelDescriptor(tt.base)
		require.Equal(t, tt.context, descriptor.ContextWindow)
		require.Equal(t, tt.base, descriptor.Slug)
		require.NotNil(t, descriptor.DefaultReasoningLevel)
		require.True(t, descriptor.SupportsParallelToolCalls)
	}
	for _, model := range []string{"gpt-6.2-sol-preview", "gpt-6.2-astra-law"} {
		require.Empty(t, openai.GPT62ModelID(model))
	}
}

func TestGPT62RejectsDisabledModelEffortAndDropsUnsupportedSampling(t *testing.T) {
	for _, base := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		require.ErrorContains(t, validateGPT61CompatRequest([]byte(`{"model":"`+base+`-none"}`), base), "does not support reasoning effort")
		require.ErrorContains(t, validateGPT61CompatRequest([]byte(`{"model":"`+base+`-minimal"}`), base), "does not support reasoning effort")
		body, changed, err := normalizeGPT6ResponsesSampling([]byte(`{"temperature":0.7,"top_p":0.9,"logprobs":true,"top_logprobs":3,"include":["reasoning.encrypted_content","message.output_text.logprobs"]}`), base)
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"include":["reasoning.encrypted_content"]}`, string(body))
	}
}
