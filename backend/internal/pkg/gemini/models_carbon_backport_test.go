package gemini

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiCarbonNativeFallbackModels(t *testing.T) {
	require.True(t, HasFallbackModel("gemini-4-carbon"))
	require.True(t, HasFallbackModel("models/gemini-4-carbon"))
	found := false
	for _, model := range FallbackModelsList().Models {
		if model.Name != "models/gemini-4-carbon" {
			continue
		}
		found = true
		require.Contains(t, model.SupportedGenerationMethods, "generateContent")
		require.Contains(t, model.SupportedGenerationMethods, "streamGenerateContent")
	}
	require.True(t, found)
}
