package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicMessagesCredentialGuardRejectsForeignOAuthWithoutAccountPenalty(t *testing.T) {
	tests := []struct {
		platform string
		accountType string
		allowed bool
	}{
		{PlatformOpenAI, AccountTypeOAuth, false},
		{PlatformOpenAI, AccountTypeSetupToken, false},
		{PlatformGrok, AccountTypeOAuth, false},
		{PlatformKimi, AccountTypeOAuth, false},
		{PlatformAnthropic, AccountTypeOAuth, true},
		{PlatformGemini, AccountTypeOAuth, true},
		{PlatformAntigravity, AccountTypeOAuth, true},
		{PlatformOpenAI, AccountTypeAPIKey, true},
		{PlatformDeepseek, AccountTypeAPIKey, true},
	}
	for _, tt := range tests {
		account := &Account{Platform: tt.platform, Type: tt.accountType}
		err := checkAnthropicMessagesCredential(account)
		if tt.allowed {
			require.NoError(t, err)
			continue
		}
		var failover *UpstreamFailoverError
		require.True(t, errors.As(err, &failover))
		require.Equal(t, GatewayFailureScopeRequest, failover.Scope)
		require.Equal(t, anthropicMessagesForeignOAuthReason, failover.Reason)
		require.True(t, failover.ShouldRetryNextAccount())
		require.False(t, failover.IsCredentialFailure())
	}
}
