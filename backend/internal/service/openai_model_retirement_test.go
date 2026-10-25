package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRetiredGPT55UsesActualCredentialNotGatewayAPIKey(t *testing.T) {
	for _, tc := range []struct {
		accountType, model string
		retired            bool
	}{
		{AccountTypeOAuth, "gpt-5.5", true},
		{AccountTypeSetupToken, "gpt-5.5-high", true},
		{AccountTypeOAuth, "gpt-5.5-pro", true},
		{AccountTypeAPIKey, "gpt-5.5", false},
		{AccountTypeOAuth, "gpt-6-sol", false},
	} {
		account := &Account{Platform: PlatformOpenAI, Type: tc.accountType}
		require.Equal(t, tc.retired, isRetiredOpenAISubscriptionModel(account, tc.model), tc)
	}
}

func TestRetiredGPT55OtherPlatformUnaffected(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}
	require.False(t, isRetiredOpenAISubscriptionModel(account, "gpt-5.5"))
}

func TestRetiredGPT55CatalogKeepsAPIKeyPath(t *testing.T) {
	oauth := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
	apikey := Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive}
	available := []string{"gpt-5.5", "gpt-6-sol"}
	oauthOnly := filterRetiredOpenAIModelIDs([]Account{oauth}, available)
	require.NotContains(t, oauthOnly, "gpt-5.5")
	mixed := filterRetiredOpenAIModelIDs([]Account{oauth, apikey}, available)
	// API-key accounts may continue to serve GPT-5.5.
	require.Contains(t, mixed, "gpt-5.5")
}
