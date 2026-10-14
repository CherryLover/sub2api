//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGPT61SolSubscriptionLongContextExemption(t *testing.T) {
	for _, tc := range []struct {
		typ   string
		model string
		want  bool
	}{
		{AccountTypeOAuth, "gpt-6.1-sol", true},
		{AccountTypeSetupToken, "gpt-6.1-sol-high", true},
		{AccountTypeAPIKey, "gpt-6.1-sol", false},
		{AccountTypeOAuth, "gpt-5.6-sol", false},
		{AccountTypeOAuth, "gpt-6.1-sol-unknown", false},
	} {
		account := &Account{Type: tc.typ, Platform: PlatformOpenAI}
		result := &OpenAIForwardResult{Model: tc.model, UpstreamModel: tc.model}
		require.Equal(t, tc.want, openAISubscriptionLongContextExempt(account, result), tc.model)
	}
}
