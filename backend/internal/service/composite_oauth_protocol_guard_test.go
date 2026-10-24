package service

import (
	"context"
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


func TestBuildUpstreamRequestRejectsForeignOAuthBeforeNetworkRequest(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	request, forwardedBody, err := svc.buildUpstreamRequest(
		context.Background(), nil, account, []byte(`{"model":"gpt-6"}`),
		"never-send-this-token", "oauth", "gpt-6", false, false,
	)
	require.Nil(t, request)
	require.Nil(t, forwardedBody)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, GatewayFailureScopeRequest, failover.Scope)
}
