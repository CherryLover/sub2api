package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKimiCodingURLRecognizesOfficialHosts(t *testing.T) {
	for _, host := range []string{"api.kimi.com", "api.kimi.ai", "API.KIMI.AI", "api.kimi.ai:443"} {
		for _, path := range []string{"/coding", "/coding/", "/coding/v1", "/coding/v1/"} {
			baseURL := "https://" + host + path
			require.True(t, isKimiCodingBaseURL(baseURL), baseURL)
			account := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"account_mode": AccountModeCoding,
				"base_url":     baseURL,
			}}
			require.Equal(t, PlatformKimi, account.GetCodingPlanProvider())
			require.Equal(t, "https://"+host+"/coding/v1/usages", kimiQuotaURL(baseURL))
		}
	}
}

func TestKimiCodingURLRejectsUntrustedOriginsAndPaths(t *testing.T) {
	for _, raw := range []string{
		"https://api.kimi.ai.untrusted.example/coding/v1",
		"https://relay.example/api.kimi.com/coding/v1",
		"https://api.kimi.ai@relay.example/coding/v1",
		"https://user@api.kimi.ai/coding/v1",
		"https://api.kimi.ai/coding-other",
		"https://api.kimi.com/coding/v10",
		"https://api.kimi.ai/coding/v1/usages",
		"https://api.kimi.ai/coding/v1?target=other",
		"https://api.kimi.ai/coding/v1?",
		"https://api.kimi.ai/coding/v1#",
		"https://api.kimi.ai/%63oding/v1",
		"https://api.kimi.ai/coding%2fv1",
		"https://api.kimi.ai/coding/../coding/v1",
		"https://api.kimi.ai:8443/coding/v1",
		"http://api.kimi.ai/coding/v1",
	} {
		require.False(t, isKimiCodingBaseURL(raw), raw)
		account := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"account_mode": AccountModeCoding,
			"base_url":     raw,
		}}
		require.Empty(t, account.GetCodingPlanProvider(), raw)
	}
}
