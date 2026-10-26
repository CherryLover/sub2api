package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type compositeOAuthCandidateRepository struct {
	AccountRepository
	accounts []Account
}

func (s *compositeOAuthCandidateRepository) ListModelAvailabilityCandidates(_ context.Context, _ *int64, _ []string, _ bool) ([]Account, error) {
	return s.accounts, nil
}

func compositeOAuthAccount(platform, accountType, publicModel string) Account {
	return Account{
		Platform: platform,
		Type:     accountType,
		Credentials: map[string]any{
			"model_mapping": map[string]any{publicModel: "upstream-model"},
		},
	}
}

func TestCompositeOAuthAmbiguityBlocksMisrouteBeforeSelectingAccount(t *testing.T) {
	repo := &compositeOAuthCandidateRepository{
		accounts: []Account{
			compositeOAuthAccount(PlatformOpenAI, AccountTypeOAuth, "gpt-6"),
			compositeOAuthAccount(PlatformDeepseek, AccountTypeAPIKey, "gpt-6"),
		},
	}
	resolver := ProvideCompositeRouteResolver(nil, repo)
	decision, err := resolver.Resolve(context.Background(), 10, "gpt-6", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.False(t, decision.Matched)
	require.True(t, decision.OAuthAmbiguous())
	require.Contains(t, decision.AmbiguousMessage(), "explicit composite route")
}

func TestCompositeOAuthAmbiguityAllowsExplicitRoute(t *testing.T) {
	repo := &compositeOAuthCandidateRepository{
		accounts: []Account{
			compositeOAuthAccount(PlatformOpenAI, AccountTypeOAuth, "gpt-6"),
			compositeOAuthAccount(PlatformDeepseek, AccountTypeAPIKey, "gpt-6"),
		},
	}
	routeRepo := compositeRouteRepoStub{
		routes: []CompositeModelRoute{{
			ID: 1, GroupID: 10, PublicModel: "gpt-6",
			MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI,
			Endpoint: CompositeRouteEndpointResponses, Enabled: true,
		}},
	}
	resolver := ProvideCompositeRouteResolver(routeRepo, repo)
	decision, err := resolver.Resolve(context.Background(), 10, "gpt-6", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.False(t, decision.OAuthAmbiguous())
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
}

func TestCompositeOverlappingAPIKeysKeepLegacyModelDetection(t *testing.T) {
	repo := &compositeOAuthCandidateRepository{
		accounts: []Account{
			compositeOAuthAccount(PlatformOpenAI, AccountTypeAPIKey, "gpt-6"),
			compositeOAuthAccount(PlatformDeepseek, AccountTypeAPIKey, "gpt-6"),
		},
	}
	resolver := ProvideCompositeRouteResolver(nil, repo)
	decision, err := resolver.Resolve(context.Background(), 10, "gpt-6", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.False(t, decision.OAuthAmbiguous())
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
}

func TestCompositeOAuthWithNoConflictingMappingKeepsModelDetection(t *testing.T) {
	repo := &compositeOAuthCandidateRepository{
		accounts: []Account{compositeOAuthAccount(PlatformOpenAI, AccountTypeOAuth, "gpt-6")},
	}
	resolver := ProvideCompositeRouteResolver(nil, repo)
	decision, err := resolver.Resolve(context.Background(), 10, "gpt-6", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.False(t, decision.OAuthAmbiguous())
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
}
