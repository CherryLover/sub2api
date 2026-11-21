//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

// Account eligibility must use the exact upstream model after GPT-6.2 alias
// normalization rather than routing a restricted session into the same model.
func TestGPT62WSAccountEligibilityUsesCanonicalUpstreamModel(t *testing.T) {
	svc := newWSTurnEligibilityService(&stubGatewayCache{})
	for _, base := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		account := newWSTurnEligibilityAccount()
		account.Type = AccountTypeOAuth
		account.Credentials = map[string]any{"access_token": "token"}
		account.Extra = map[string]any{modelRateLimitsKey: wsTurnModelRateLimits(base)}
		requested := base + "-high"
		require.Equal(t, base, openAIWSUpstreamModelForAccount(account, requested))
		reason, err := svc.EnforceOpenAIWSTurnAccountEligibility(context.Background(), account, nil, "", requested)
		require.Equal(t, OpenAIWSTurnAccountIneligibleModelRateLimited, reason)
		requireWSTurnCloseError(t, err, OpenAIWSTurnAccountIneligibleModelRateLimited)
	}
}

func TestGPT62CompositeExplicitRoutingPreservesCanonicalWSModel(t *testing.T) {
	for _, base := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		resolver := NewCompositeRouteResolver(compositeRouteRepoStub{routes: []CompositeModelRoute{{
			ID: 910, GroupID: 7, PublicModel: "alias/" + base, MatchType: CompositeRouteMatchExact,
			TargetPlatform: PlatformOpenAI, UpstreamModel: base + "-high", Endpoint: CompositeRouteEndpointResponses,
			Priority: 100, Enabled: true,
		}}})
		decision, err := resolver.Resolve(context.Background(), 7, "alias/"+base, CompositeRouteEndpointResponses)
		require.NoError(t, err)
		require.True(t, decision.Matched)
		require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
		require.Equal(t, base+"-high", decision.UpstreamModel)
		require.Equal(t, base, normalizeCodexModel(decision.UpstreamModel))
		require.Equal(t, base, normalizeKnownOpenAICodexModel(decision.UpstreamModel))
	}
}
