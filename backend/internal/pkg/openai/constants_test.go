package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsIncludeBareGPT56Alias(t *testing.T) {
	require.Contains(t, DefaultModelIDs(), "gpt-5.6")
}

func TestDefaultModelsIncludeGPT6Astra(t *testing.T) {
	require.Contains(t, DefaultModelIDs(), "gpt-6-astra")
	require.Contains(t, DefaultModelIDs(), "gpt-6")
	var displayName string
	for _, model := range DefaultModels {
		if model.ID == "gpt-6-astra" {
			displayName = model.DisplayName
			break
		}
	}
	require.Equal(t, "GPT-6 Astra", displayName)
}

func TestDefaultModelsIncludeGPT6SolAndLuna(t *testing.T) {
	require.Contains(t, DefaultModelIDs(), "gpt-6-sol")
	require.Contains(t, DefaultModelIDs(), "gpt-6-luna")
	require.True(t, IsGPT6SolOrLunaModelSpelling("openai/gpt-6-sol-max"))
	require.True(t, IsGPT6SolOrLunaModelSpelling("gpt-6-luna-openai-compact"))
	require.False(t, IsGPT6SolOrLunaModelSpelling("gpt-6-astra"))
}

func TestDefaultModelsPreferConcreteGPT56SolForAccountTests(t *testing.T) {
	require.NotEmpty(t, DefaultModels)
	require.Equal(t, "gpt-5.6-sol", DefaultModels[0].ID)
}
