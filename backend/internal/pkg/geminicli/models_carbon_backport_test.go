package geminicli

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiCarbonCuratedCLIModelWithoutLegacyGoogleOneAccess(t *testing.T) {
	found := false
	for _, model := range DefaultModels {
		if model.ID == "gemini-4-carbon" {
			found = true
			require.Equal(t, "Gemini 4 Carbon", model.DisplayName)
		}
	}
	require.True(t, found)
	for _, model := range GoogleOneModels {
		require.NotEqual(t, "gemini-4-carbon", model.ID)
	}
}
