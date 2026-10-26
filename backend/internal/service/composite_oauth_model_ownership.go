package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

// ProvideCompositeRouteResolver adds configured-account ownership checking to
// the production resolver without changing the standalone one-argument
// constructor used by legacy/isolated callers.
func ProvideCompositeRouteResolver(routes CompositeModelRouteRepository, accounts AccountRepository) *CompositeRouteResolver {
	resolver := NewCompositeRouteResolver(routes)
	if accounts == nil {
		return resolver
	}
	// Bound database lookups on the composite routing hot path. The TTL is
	// deliberately short: newly edited model mappings converge promptly.
	cache := gocache.New(10*time.Second, time.Minute)
	resolver.SetModelOwnershipResolver(func(ctx context.Context, groupID int64, model string) (CompositeModelOwnership, error) {
		model = strings.TrimSpace(model)
		if groupID <= 0 || model == "" {
			return CompositeModelOwnership{}, nil
		}
		key := fmt.Sprintf("%d:%s", groupID, model)
		if cached, found := cache.Get(key); found {
			if ownership, ok := cached.(CompositeModelOwnership); ok {
				return ownership, nil
			}
		}
		concretePlatforms := []string{
			PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity,
			PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek,
		}
		candidates, err := accounts.ListModelAvailabilityCandidates(ctx, &groupID, concretePlatforms, false)
		if err != nil {
			return CompositeModelOwnership{}, err
		}
		owner := CompositeModelOwnership{}
		platforms := make(map[string]struct{})
		for _, account := range candidates {
			if account.Credentials == nil || !isConcreteRequestPlatform(account.Platform) {
				continue
			}
			if mapped := stringMappingFromRaw(account.Credentials["model_mapping"])[model]; strings.TrimSpace(mapped) == "" {
				continue
			}
			platforms[account.Platform] = struct{}{}
			if account.IsOAuth() && isOpenAIGatewayProtocolPlatform(account.Platform) {
				owner.OpenAIGatewayOAuthClaimed = true
			}
		}
		if len(platforms) == 1 {
			owner.Matched = true
			for platform := range platforms {
				owner.TargetPlatform = platform
			}
		} else if len(platforms) > 1 {
			owner.Ambiguous = true
		}
		cache.SetDefault(key, owner)
		return owner, nil
	})
	return resolver
}
