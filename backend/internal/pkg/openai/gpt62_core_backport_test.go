package openai

import (
  "testing"
  "github.com/stretchr/testify/require"
)

func TestGPT62CatalogEffortAndOfficialInstructions(t *testing.T) {
 for _, base := range []string{"gpt-6.2-sol","gpt-6.2-astra"} {
  require.Contains(t, DefaultModelIDs(), base)
  for _, suffix := range []string{"","-none","-minimal","-low","-medium","-high","-xhigh","-max","-openai-compact"} {
   model := "openai/"+base+suffix
   require.Equal(t,base,GPT62ModelID(model))
   require.True(t,RequiresResponsesForToolCalls(model))
   require.NotEmpty(t,CodexBaseInstructionsForModel(model))
  }
  for _,effort:=range []string{"none","minimal"} {require.Error(t,ValidateGPT61ReasoningEffort(base,effort))}
  for _,effort:=range []string{"low","medium","high","xhigh","max"} {require.NoError(t,ValidateGPT61ReasoningEffort(base,effort))}
 }
 for _, model:=range []string{"gpt-6.2","gpt-6.2-astra-preview","gpt-6.2-sol-foo"} {require.Empty(t,GPT62ModelID(model))}
}
