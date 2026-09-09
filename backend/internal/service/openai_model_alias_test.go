package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeKnownOpenAICodexModel_BareGPT56RoutesToSol(t *testing.T) {
	tests := map[string]string{
		"gpt-5.6":            "gpt-5.6-sol",
		"openai/gpt-5.6":     "gpt-5.6-sol",
		"gpt5.6":             "gpt-5.6-sol",
		"gpt-5.6-high":       "gpt-5.6-sol",
		"gpt-5.6-max":        "gpt-5.6-sol",
		"gpt-5.6-2026-07-09": "gpt-5.6-sol",
		"openai/gpt-5.6-max": "gpt-5.6-sol",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
}

func TestNormalizeKnownOpenAICodexModel_BareGPT6RoutesToAstra(t *testing.T) {
	tests := map[string]string{
		"gpt-6":              "gpt-6-astra",
		"gpt-6-astra":        "gpt-6-astra",
		"openai/gpt-6":       "gpt-6-astra",
		"OPENAI/GPT-6_ASTRA": "gpt-6-astra",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
}

func TestIsOpenAIGPT6AstraModel(t *testing.T) {
	require.True(t, isOpenAIGPT6AstraModel("gpt-6"))
	require.True(t, isOpenAIGPT6AstraModel("gpt-6-astra"))
	require.True(t, isOpenAIGPT6AstraModel("openai/gpt-6-astra-2026-09-01"))
	require.False(t, isOpenAIGPT6AstraModel("gpt-5.6-sol"))
}

func TestUsageBillingModelCandidates_BareGPT56IncludesSol(t *testing.T) {
	require.Equal(t,
		[]string{"gpt-5.6", "gpt-5.6-sol"},
		usageBillingModelCandidates("gpt-5.6"),
	)
	require.Equal(t,
		[]string{"openai/gpt-5.6", "gpt-5.6", "gpt-5.6-sol"},
		usageBillingModelCandidates("openai/gpt-5.6"),
	)
}
