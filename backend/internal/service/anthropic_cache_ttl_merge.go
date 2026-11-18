package service

import "github.com/tidwall/gjson"

// mergeAnthropicCacheCreationSnapshot merges cumulative Anthropic cache usage.
// SSE deltas may omit an aggregate, contain null, or carry a zero placeholder;
// none of those cases should erase previously observed usage. TTL buckets may
// arrive independently and are authoritative only if no positive aggregate is
// available. Repeat cumulative frames are idempotent.
func mergeAnthropicCacheCreationSnapshot(node gjson.Result, usage *ClaudeUsage) {
	if usage == nil || !node.IsObject() {
		return
	}
	aggregate := node.Get("cache_creation_input_tokens")
	if aggregate.Exists() && aggregate.Type != gjson.Null {
		value := max(int(aggregate.Int()), 0)
		if value > 0 {
			usage.CacheCreationInputTokens = value
			usage.cacheCreationInputTokensPresent = true
			usage.cacheCreationInputTokensFromDetails = false
		} else if usage.CacheCreationInputTokens <= 0 {
			usage.CacheCreationInputTokens = 0
			usage.cacheCreationInputTokensPresent = true
			usage.cacheCreationInputTokensFromDetails = false
		}
	}
	hasDetail := false
	if part := node.Get("cache_creation.ephemeral_5m_input_tokens"); part.Exists() && part.Type != gjson.Null {
		usage.CacheCreation5mTokens = max(int(part.Int()), 0)
		hasDetail = true
	}
	if part := node.Get("cache_creation.ephemeral_1h_input_tokens"); part.Exists() && part.Type != gjson.Null {
		usage.CacheCreation1hTokens = max(int(part.Int()), 0)
		hasDetail = true
	}
	if hasDetail && (!usage.cacheCreationInputTokensPresent || usage.cacheCreationInputTokensFromDetails || usage.CacheCreationInputTokens <= 0) {
		usage.CacheCreationInputTokens = usage.CacheCreation5mTokens + usage.CacheCreation1hTokens
		usage.cacheCreationInputTokensFromDetails = true
	}
}
