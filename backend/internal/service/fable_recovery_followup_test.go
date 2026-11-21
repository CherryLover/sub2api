package service

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fableRecoveryCASRepo struct {
	*sessionWindowMockRepo
	live  map[string]any
	calls int
	err   error
}

func (r *fableRecoveryCASRepo) ClearModelRateLimitSnapshotIfUnchanged(_ context.Context, _ int64, scope string, expected map[string]any) (bool, error) {
	r.calls++
	if r.err != nil {
		return false, r.err
	}
	if !reflect.DeepEqual(r.live[scope], expected) {
		return false, nil
	}
	delete(r.live, scope)
	return true, nil
}

func TestFableRecoveryHeadersUseExactScopeSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, utilization, status, reason string
		concurrent, fail                  bool
		wantCalls                         int
		wantClear                         bool
	}{
		{name: "recovered zero", utilization: "0", wantCalls: 1, wantClear: true},
		{name: "recovered below threshold", utilization: "0.5", wantCalls: 1, wantClear: true},
		{name: "missing observation"},
		{name: "invalid number", utilization: "NaN"},
		{name: "infinity", utilization: "+Inf"},
		{name: "negative", utilization: "-0.1"},
		{name: "near exhaustion", utilization: "0.95"},
		{name: "still exhausted", utilization: "1"},
		{name: "rejected status", utilization: "0", status: "rejected"},
		{name: "credit restriction", utilization: "0", reason: anthropicFableCreditsRequiredReason},
		{name: "concurrent replacement", utilization: "0", concurrent: true, wantCalls: 1},
		{name: "database failure", utilization: "0", fail: true, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			account := &Account{ID: 72, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
			reason := tc.reason
			if reason == "" {
				reason = anthropicFableWindowReason
			}
			setAccountModelRateLimitSnapshot(account, anthropicFableRateLimitKey, now.Add(time.Hour), reason, now)
			setAccountModelRateLimitSnapshot(account, "unrelated-model", now.Add(2*time.Hour), "admin_limit", now)
			original := account.Extra
			scopes, ok := account.Extra[modelRateLimitsKey].(map[string]any)
			require.True(t, ok)
			live := map[string]any{}
			for k, v := range scopes {
				live[k] = v
			}
			if tc.concurrent {
				fresh := map[string]any{}
				observedScope, ok := scopes[anthropicFableRateLimitKey].(map[string]any)
				require.True(t, ok)
				for k, v := range observedScope {
					fresh[k] = v
				}
				fresh["reason"] = anthropicFableCreditsRequiredReason // Same reset time is still a new restriction.
				live[anthropicFableRateLimitKey] = fresh
			}
			repo := &fableRecoveryCASRepo{sessionWindowMockRepo: &sessionWindowMockRepo{}, live: live}
			if tc.fail {
				repo.err = errors.New("synthetic database failure")
			}
			headers := http.Header{}
			if tc.utilization != "" {
				headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", tc.utilization)
			}
			if tc.status != "" {
				headers.Set("anthropic-ratelimit-unified-7d_oi-status", tc.status)
			}
			newRateLimitServiceForTest(repo).samplePassiveUsageFromHeaders(context.Background(), account, headers)
			require.Equal(t, tc.wantCalls, repo.calls)
			require.Equal(t, !tc.wantClear, account.isRateLimitActiveForKey(anthropicFableRateLimitKey))
			_, livePresent := live[anthropicFableRateLimitKey]
			require.Equal(t, !tc.wantClear, livePresent)
			require.Contains(t, live, "unrelated-model")
			originalScopes, ok := original[modelRateLimitsKey].(map[string]any)
			require.True(t, ok)
			require.Contains(t, originalScopes, anthropicFableRateLimitKey, "shared snapshots must not be mutated")
		})
	}
}
