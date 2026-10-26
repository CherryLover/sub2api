package service

import "strings"

// GPT-5.5 ceased to be available to ChatGPT-signed Codex OAuth/setup-token
// accounts on 2026-10-14. API-key upstreams may still serve the model.
func isRetiredOpenAISubscriptionModel(account *Account, upstreamModel string) bool {
	if account == nil || !account.IsOpenAI() || (!account.UsesOpenAICodexProtocol() && !account.IsShadow()) {
		return false
	}
	model := canonicalizeOpenAIModelAliasSpelling(upstreamModel)
	return model == "gpt-5.5" || strings.HasPrefix(model, "gpt-5.5-")
}

func supportsOpenAIRequestModel(account *Account, requestedModel string, requireCompact bool) bool {
	if account == nil {
		return false
	}
	target := resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, requireCompact)
	return !isRetiredOpenAISubscriptionModel(account, target) && account.IsModelSupported(requestedModel)
}

// Preserve GPT-5.5 in public catalogs if any API-key account can serve it,
// but hide names that can only reach retired ChatGPT-backed upstreams.
func filterRetiredOpenAIModelIDs(accounts []Account, models []string) []string {
	result := make([]string, 0, len(models))
	for _, model := range models {
		retired := false
		supportedByActivePath := false
		for i := range accounts {
			account := &accounts[i]
			if !account.IsOpenAI() {
				continue
			}
			target := resolveOpenAIAccountUpstreamModelForRequest(account, model, false)
			if isRetiredOpenAISubscriptionModel(account, target) {
				retired = true
			} else if account.IsModelSupported(model) {
				supportedByActivePath = true
			}
		}
		if !retired || supportedByActivePath {
			result = append(result, model)
		}
	}
	return result
}
