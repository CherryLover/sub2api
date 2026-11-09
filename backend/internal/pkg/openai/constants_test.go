package openai

import (
	"encoding/json"
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

func TestGPT61SolModelRecognizesAliasesAndRejectsDisabledReasoning(t *testing.T) {
	require.Contains(t, DefaultModelIDs(), "gpt-6.1-sol")
	for _, name := range []string{"gpt-6.1-sol", "openai/gpt-6.1-sol-xhigh", "gpt-6.1-sol-openai-compact"} {
		require.True(t, IsGPT61SolModelSpelling(name), name)
	}
	require.False(t, IsGPT61SolModelSpelling("gpt-6.1-sol-fictional"))
	for _, effort := range []string{"none", "minimal"} {
		require.Error(t, ValidateGPT61SolReasoningEffort("openai/gpt-6.1-sol", effort))
	}
	require.NoError(t, ValidateGPT61SolReasoningEffort("gpt-6.1-sol", "xhigh"))
	require.NoError(t, ValidateGPT61SolReasoningEffort("gpt-6-sol", "none"))
}

func TestDefaultModelsPreferConcreteGPT56SolForAccountTests(t *testing.T) {
	require.NotEmpty(t, DefaultModels)
	require.Equal(t, "gpt-5.6-sol", DefaultModels[0].ID)
}

func TestValidateGPT61TerraReasoningEffort(t *testing.T) {
	for _, model := range []string{"gpt-6.1-terra", "openai/gpt-6.1-terra-minimal"} {
		require.Error(t, ValidateGPT61ReasoningEffort(model, "minimal"), model)
		require.NoError(t, ValidateGPT61ReasoningEffort(model, "none"), model)
		require.NoError(t, ValidateGPT61ReasoningEffort(model, "max"), model)
	}
	require.NoError(t, ValidateGPT61ReasoningEffort("gpt-6.1-sol", "max"))
	require.NoError(t, ValidateGPT61ReasoningEffort("gpt-5.6-sol", "minimal"))
}

func TestDefaultModelsIncludeGPTImage25And26(t *testing.T) {
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		require.Contains(t, DefaultModelIDs(), model)
	}
}

func TestCodexBaseInstructionsForTerraOfficialDescriptor(t *testing.T) {
	var descriptor struct {
		ModelMessages struct {
			InstructionsTemplate string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	require.NoError(t, json.Unmarshal(CodexGPT61TerraMetadata, &descriptor))
	require.NotEmpty(t, descriptor.ModelMessages.InstructionsTemplate)
	require.Equal(t, descriptor.ModelMessages.InstructionsTemplate, CodexBaseInstructionsForModel("openai/GPT-6.1-TERRA"))
	require.NotEqual(t, CodexBaseInstructionsForModel("gpt-5.5"), CodexBaseInstructionsForModel("gpt-6.1-terra"))
}
