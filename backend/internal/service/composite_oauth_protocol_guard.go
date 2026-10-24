package service

import "net/http"

func isOpenAIGatewayProtocolPlatform(platform string) bool {
	switch platform {
	case PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek:
		return true
	default:
		return false
	}
}

// anthropicMessagesCredentialAllowed checks whether an account credential can
// be sent to an Anthropic Messages upstream. OAuth/SetupToken credentials from
// providers using OpenAI's gateway must never be sent to Anthropic.
func anthropicMessagesCredentialAllowed(account *Account) bool {
	if account == nil || !account.IsOAuth() {
		return true
	}
	return !isOpenAIGatewayProtocolPlatform(account.Platform)
}

const anthropicMessagesForeignOAuthReason GatewayFailureReason = "anthropic_messages_foreign_oauth"

// The error only advances the account failover loop; it does not penalize the
// account because no upstream request was made.
func checkAnthropicMessagesCredential(account *Account) error {
	if anthropicMessagesCredentialAllowed(account) {
		return nil
	}
	return &UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
		Reason:     anthropicMessagesForeignOAuthReason,
		Scope:      GatewayFailureScopeRequest,
	}
}
