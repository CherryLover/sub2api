//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const bareAntigravity429 = `{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`

func newAntigravityCooldownTestAccount(plan any) *Account {
	return &Account{
		ID: 9701, Platform: PlatformAntigravity, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"plan_type": plan,
			"model_mapping": map[string]any{
				"gemini-4-argon-high": "gemini-4-argon-high",
				"gemini-4-argon-low":  "gemini-4-argon-low",
				"client-argon":        "gemini-4-argon-high",
				"gemini-3.8-flash":    "gemini-3.8-flash",
				"gemini-2.5-pro":      "gemini-2.5-pro",
				"gemini-3-pro-image":  "gemini-3-pro-image",
				"claude-sonnet-4-6":   "claude-sonnet-4-6",
			},
		},
	}
}

func antigravityCooldownSnapshot(t *testing.T, account *Account, calls []modelRateLimitCall) *Account {
	t.Helper()
	limits := make(map[string]any, len(calls))
	for _, c := range calls {
		require.Equal(t, account.ID, c.accountID)
		limits[c.modelKey] = map[string]any{"rate_limit_reset_at": c.resetAt.Format(time.RFC3339)}
	}
	account.Extra = map[string]any{modelRateLimitsKey: limits}
	payload, err := json.Marshal(account)
	require.NoError(t, err)
	var snapshot Account
	require.NoError(t, json.Unmarshal(payload, &snapshot))
	return &snapshot
}

func antigravityCooldownCallKeys(calls []modelRateLimitCall) []string {
	keys := make([]string, 0, len(calls))
	for _, call := range calls {
		keys = append(keys, call.modelKey)
	}
	return keys
}

func TestAntigravityProArgonPoolSeparatedOnQuotaReset(t *testing.T) {
	cases := []struct {
		name      string
		plan      any
		exhausted string
		same      []string
		other     []string
		separate  bool
	}{
		{"Pro Argon", "Pro", "gemini-4-argon-high", []string{"gemini-4-argon-low", "client-argon"}, []string{"gemini-3.8-flash", "gemini-2.5-pro"}, true},
		{"Pro primary Gemini", "g1-pro-tier", "gemini-3.8-flash", []string{"gemini-2.5-pro"}, []string{"gemini-4-argon-low", "client-argon"}, true},
		{"Ultra shared", "Ultra", "gemini-4-argon-high", []string{"gemini-4-argon-low", "gemini-3.8-flash"}, nil, false},
		{"Free shared", "Free", "gemini-4-argon-high", []string{"gemini-3.8-flash"}, nil, false},
		{"Unknown shared", nil, "gemini-4-argon-high", []string{"gemini-3.8-flash"}, nil, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			account := newAntigravityCooldownTestAccount(tt.plan)
			repo := &stubAntigravityAccountRepo{}
			svc := &AntigravityGatewayService{}
			resetAt := time.Now().Add(5 * time.Minute)
			require.True(t, svc.setAntigravityModelRateLimits(context.Background(), repo, account, tt.exhausted, "[test]", 429, resetAt, false))
			require.Len(t, repo.modelRateLimitCalls, 2)
			require.Equal(t, tt.exhausted, repo.modelRateLimitCalls[0].modelKey)
			if tt.separate && tt.exhausted == "gemini-4-argon-high" {
				require.Equal(t, antigravityArgonModelRateLimitKey, repo.modelRateLimitCalls[1].modelKey)
			} else {
				require.Equal(t, antigravityGeminiModelRateLimitKey, repo.modelRateLimitCalls[1].modelKey)
			}
			snapshot := antigravityCooldownSnapshot(t, account, repo.modelRateLimitCalls)
			require.False(t, snapshot.IsSchedulableForModel(tt.exhausted))
			for _, model := range tt.same {
				require.False(t, snapshot.IsSchedulableForModel(model), model)
			}
			for _, model := range tt.other {
				require.True(t, snapshot.IsSchedulableForModel(model), model)
			}
			require.True(t, snapshot.IsSchedulableForModel("gemini-3-pro-image"))
			require.True(t, snapshot.IsSchedulableForModel("claude-sonnet-4-6"))
		})
	}
}

func TestAntigravityBare429OnlyCoolsFailedModel(t *testing.T) {
	t.Setenv(antigravityFallbackSecondsEnv, "")
	for _, tt := range []struct {
		plan  any
		model string
		other string
	}{
		{"Pro", "gemini-4-argon-high", "gemini-4-argon-low"},
		{"Pro", "gemini-3.8-flash", "gemini-2.5-pro"},
		{"Ultra", "gemini-3.8-flash", "gemini-4-argon-high"},
	} {
		t.Run(tt.model+"/"+tt.other, func(t *testing.T) {
			account := newAntigravityCooldownTestAccount(tt.plan)
			repo := &stubAntigravityAccountRepo{}
			svc := &AntigravityGatewayService{accountRepo: repo}
			before := time.Now()
			res := svc.handleUpstreamError(context.Background(), "[test]", account, http.StatusTooManyRequests, http.Header{}, []byte(bareAntigravity429), tt.model, 0, "", false)
			require.Nil(t, res)
			require.Empty(t, repo.rateCalls, "bare 429 must not pause the entire account")
			require.Equal(t, []string{tt.model}, antigravityCooldownCallKeys(repo.modelRateLimitCalls))
			require.WithinDuration(t, before.Add(antigravityDefaultRateLimitDuration), repo.modelRateLimitCalls[0].resetAt, 3*time.Second)
			snapshot := antigravityCooldownSnapshot(t, account, repo.modelRateLimitCalls)
			require.False(t, snapshot.IsSchedulableForModel(tt.model))
			require.True(t, snapshot.IsSchedulableForModel(tt.other), "unrelated model must remain schedulable")
		})
	}
}

func TestAntigravity429WithResetTimeCoolsCorrectPool(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       string
	}{
		{"quotaResetDelay", string(buildGeminiRateLimitBody("30s")), antigravityArgonModelRateLimitKey},
		{"RetryInfo", `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"30s"}]}}`, antigravityArgonModelRateLimitKey},
		{"retry in text", `{"error":{"code":429,"message":"Please retry in 30s","status":"RESOURCE_EXHAUSTED"}}`, antigravityArgonModelRateLimitKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := newAntigravityCooldownTestAccount("Pro")
			repo := &stubAntigravityAccountRepo{}
			svc := &AntigravityGatewayService{accountRepo: repo}
			res := svc.handleUpstreamError(context.Background(), "[test]", account, 429, http.Header{}, []byte(tt.body), "gemini-4-argon-high", 0, "", false)
			require.Nil(t, res)
			require.Empty(t, repo.rateCalls)
			require.Equal(t, []string{"gemini-4-argon-high", tt.want}, antigravityCooldownCallKeys(repo.modelRateLimitCalls))
			snapshot := antigravityCooldownSnapshot(t, account, repo.modelRateLimitCalls)
			require.False(t, snapshot.IsSchedulableForModel("gemini-4-argon-low"))
			require.True(t, snapshot.IsSchedulableForModel("gemini-3.8-flash"))
		})
	}
}
