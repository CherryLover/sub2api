package service

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestFableRecoveryRequiresExplicitFreshEvidence(t *testing.T) {
	reset := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for _, tc := range []struct {
		name, body string
		wantClear  bool
	}{
		{"legacy missing utilization", fmt.Sprintf(`{"seven_day_overage_included":{"resets_at":%q}}`, reset), false},
		{"legacy null utilization", fmt.Sprintf(`{"seven_day_overage_included":{"utilization":null,"resets_at":%q}}`, reset), false},
		{"legacy explicit zero", fmt.Sprintf(`{"seven_day_overage_included":{"utilization":0,"resets_at":%q}}`, reset), true},
		{"legacy invalid negative", fmt.Sprintf(`{"seven_day_overage_included":{"utilization":-1,"resets_at":%q}}`, reset), false},
		{"scoped missing percent", fmt.Sprintf(`{"limits":[{"kind":"weekly_scoped","group":"weekly","scope":{"model":{"display_name":"Fable"}},"resets_at":%q}]}`, reset), false},
		{"scoped explicit zero", fmt.Sprintf(`{"limits":[{"kind":"weekly_scoped","group":"weekly","scope":{"model":{"display_name":"Fable"}},"percent":0,"resets_at":%q}]}`, reset), true},
		{"empty response", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response ClaudeUsageResponse
			require.NoError(t, json.Unmarshal([]byte(tc.body), &response))
			now := time.Now()
			usage := (&AccountUsageService{}).buildUsageInfo(&response, &now)
			account := &Account{ID: 735, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
			setAccountModelRateLimitSnapshot(account, anthropicFableRateLimitKey, now.Add(time.Hour), anthropicFableWindowReason, now)
			limits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
			require.True(t, ok)
			live := map[string]any{anthropicFableRateLimitKey: limits[anthropicFableRateLimitKey]}
			repo := &fableRecoveryCASRepo{sessionWindowMockRepo: &sessionWindowMockRepo{}, live: live}
			require.Equal(t, tc.wantClear, recoverObservedActiveFableUsage(t.Context(), repo, account, usage))
			if !tc.wantClear {
				require.Zero(t, repo.calls, "missing or invalid usage cannot prove recovery")
			}
		})
	}
}

func TestFableRecoveryDoesNotIgnoreSurpassedThreshold(t *testing.T) {
	now := time.Now()
	account := &Account{ID: 736, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	setAccountModelRateLimitSnapshot(account, anthropicFableRateLimitKey, now.Add(time.Hour), anthropicFableWindowReason, now)
	limits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	require.True(t, ok)
	repo := &fableRecoveryCASRepo{sessionWindowMockRepo: &sessionWindowMockRepo{}, live: map[string]any{anthropicFableRateLimitKey: limits[anthropicFableRateLimitKey]}}
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", "0")
	headers.Set("anthropic-ratelimit-unified-7d_oi-surpassed-threshold", "true")
	newRateLimitServiceForTest(repo).samplePassiveUsageFromHeaders(t.Context(), account, headers)
	require.Zero(t, repo.calls)
	require.True(t, account.isRateLimitActiveForKey(anthropicFableRateLimitKey))
}
