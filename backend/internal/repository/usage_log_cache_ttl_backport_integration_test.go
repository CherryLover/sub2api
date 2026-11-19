//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Postgres round-trip verification: the TTL buckets and aggregate must be
// persisted together, and an authoritative aggregate must not be overwritten
// by inconsistent upstream cache creation detail.
func TestUsageLogCacheCreationTTLRoundTrip(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	entClient := tx.Client()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := mustCreateUser(t, entClient, &service.User{Username: "usage-cache-ttl-" + suffix})
	key := mustCreateApiKey(t, entClient, &service.APIKey{UserID: user.ID, Key: "sk-ttl-" + suffix})
	account := mustCreateAccount(t, entClient, &service.Account{Name: "usage-cache-ttl-" + suffix, Platform: service.PlatformAnthropic})
	repo := newUsageLogRepositoryWithSQL(entClient, tx)

	for _, tc := range []struct {
		name              string
		total, five, hour int
		cost              float64
	}{
		{"TTL only aggregate inferred", 1000, 400, 600, 0.0068},
		{"explicit aggregate retained", 2000, 400, 600, 0.0068},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &service.UsageLog{
				UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
				RequestID: fmt.Sprintf("ttl-%s-%d", suffix, tc.total),
				Model:     "claude-opus-5-5", InputTokens: 1000, OutputTokens: 100,
				CacheCreationTokens: tc.total, CacheCreation5mTokens: tc.five,
				CacheCreation1hTokens: tc.hour, CacheCreationCost: tc.cost,
				TotalCost: tc.cost + 0.004, ActualCost: tc.cost + 0.004,
				CreatedAt: time.Now().UTC(),
			}
			inserted, err := repo.Create(ctx, log)
			require.NoError(t, err)
			require.True(t, inserted)
			require.NotZero(t, log.ID)
			saved, err := entClient.UsageLog.Get(ctx, log.ID)
			require.NoError(t, err)
			require.Equal(t, tc.total, saved.CacheCreationTokens)
			require.Equal(t, tc.five, saved.CacheCreation5mTokens)
			require.Equal(t, tc.hour, saved.CacheCreation1hTokens)
			require.InDelta(t, tc.cost, saved.CacheCreationCost, 1e-10)
			require.InDelta(t, log.TotalCost, saved.TotalCost, 1e-10)
		})
	}
}
