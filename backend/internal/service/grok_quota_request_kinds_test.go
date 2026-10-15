//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// Model discovery may complete either before or after QueryQuota returns.
// Callers must drain background work before asserting this endpoint-specific
// contract, so an extra inference POST cannot be mistaken for a models GET.
func assertGrokQuotaRequestKinds(t *testing.T, requests []*http.Request, wantActive, wantModels int) {
	t.Helper()
	billingQueries := make(map[string]int)
	activeCalls, modelCalls := 0, 0
	for _, req := range requests {
		require.NotNil(t, req)
		switch req.URL.Path {
		case "/v1/billing":
			require.Equal(t, http.MethodGet, req.Method)
			billingQueries[req.URL.RawQuery]++
		case "/v1/models":
			require.Equal(t, http.MethodGet, req.Method)
			modelCalls++
		case "/v1/responses":
			require.Equal(t, http.MethodPost, req.Method)
			activeCalls++
		default:
			t.Fatalf("unexpected upstream request: %s %s", req.Method, req.URL.RequestURI())
		}
	}
	require.Equal(t, map[string]int{"": 1, "format=credits": 1}, billingQueries,
		"weekly and monthly billing must each be queried exactly once")
	require.Equal(t, wantActive, activeCalls, "paid quota queries must not consume inference quota")
	require.Equal(t, wantModels, modelCalls, "model discovery must respect snapshot freshness")
	require.Len(t, requests, 2+wantActive+wantModels)
}

func TestGrokQuotaServiceQueryQuotaPaidBillingModelDiscoveryCache(t *testing.T) {
	t.Parallel()
	for index, tc := range []struct {
		name       string
		age        time.Duration
		hasCache   bool
		wantModels int
	}{
		{name: "missing snapshot", wantModels: 1},
		{name: "fresh snapshot", hasCache: true, age: time.Minute, wantModels: 0},
		{name: "expired snapshot", hasCache: true, age: grokObservedModelsTTL + time.Minute, wantModels: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				account := healthyGrokQuotaOAuthAccount(91001560 + int64(index))
				if tc.hasCache {
					account.Extra = map[string]any{
						grokObservedModelsExtraKey: grokObservedModelsSnapshot{
							Models:    []string{"grok-4.5"},
							FetchedAt: time.Now().Add(-tc.age).UTC().Format(time.RFC3339),
						},
					}
				}
				repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
					accountsByID: map[int64]*Account{account.ID: account},
				}}
				percent := 25.0
				upstream := &grokHybridUpstream{weeklyUsagePercent: &percent}
				svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream, nil)

				result, err := svc.QueryQuota(context.Background(), account.ID)
				require.NoError(t, err)
				require.Equal(t, "billing_probe", result.Source)
				require.NotNil(t, result.Billing)
				require.Nil(t, result.Snapshot)
				require.Empty(t, result.Model)
				synctest.Wait()

				requests, _ := upstream.snapshot()
				assertGrokQuotaRequestKinds(t, requests, 0, tc.wantModels)
				snapshot := parseGrokObservedModels(account.Extra)
				require.NotNil(t, snapshot)
				require.Equal(t, []string{"grok-4.5"}, snapshot.Models)
				_, inFlight := grokObservedModelsFlight.Load(account.ID)
				require.False(t, inFlight, "background work must not leak into another test iteration")
			})
		})
	}
}
