package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGPT61SolModelAliasNormalization(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "openai/gpt-6.1-sol-xhigh", "gpt-6.1-sol-openai-compact"} {
		require.Equal(t, "gpt-6.1-sol", normalizeKnownOpenAICodexModel(model))
	}
	require.True(t, isOpenAIGPT6Model("openai/gpt-6.1-sol-high"))
}

func TestGPT61SolCompatReasoningValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    bool
	}{
		{"none reasoning object", `{"model":"gpt-6.1-sol","reasoning":{"effort":"none"}}`, true},
		{"minimal field", `{"model":"gpt-6.1-sol","reasoning_effort":"minimal"}`, true},
		{"suffix none", `{"model":"gpt-6.1-sol-none"}`, true},
		{"disabled thinking", `{"model":"gpt-6.1-sol","thinking":{"type":"disabled"}}`, true},
		{"valid effort", `{"model":"gpt-6.1-sol","reasoning":{"effort":"max"}}`, false},
		{"other model unaffected", `{"model":"gpt-6-sol","reasoning_effort":"none"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := "gpt-6.1-sol"
			if tc.name == "other model unaffected" {
				model = "gpt-6-sol"
			}
			err := validateGPT61SolCompatRequest([]byte(tc.body), model)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
