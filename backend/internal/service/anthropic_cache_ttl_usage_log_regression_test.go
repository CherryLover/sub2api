//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// A provider can send only cache_creation.ephemeral_* without the aggregate.
// The persisted aggregate, TTL buckets and billed amounts must remain coherent.
func TestAnthropicCacheTTLDetailOnlyPersistsConsistentUsageLog(t *testing.T) {
	for _, tc := range []struct {
		name      string
		usage     ClaudeUsage
		aggregate int
	}{
		{"5m and 1h only", ClaudeUsage{InputTokens: 1000, OutputTokens: 100, CacheCreation5mTokens: 400, CacheCreation1hTokens: 600}, 1000},
		{"explicit aggregate", ClaudeUsage{InputTokens: 1000, OutputTokens: 100, CacheCreationInputTokens: 2000, CacheCreation5mTokens: 400, CacheCreation1hTokens: 600}, 2000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
			svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
			err := svc.RecordUsage(context.Background(), &RecordUsageInput{
				Result: &ForwardResult{RequestID: "ttl_log_consistency", Model: "claude-opus-5-5", Usage: tc.usage, Duration: time.Second},
				APIKey: &APIKey{ID: 501, Quota: 100}, User: &User{ID: 601}, Account: &Account{ID: 701, Platform: PlatformAnthropic},
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			log := usageRepo.lastLog
			require.Equal(t, tc.aggregate, log.CacheCreationTokens)
			require.Equal(t, 400, log.CacheCreation5mTokens)
			require.Equal(t, 600, log.CacheCreation1hTokens)
			require.Greater(t, log.CacheCreationCost, 0.0)
			require.NotNil(t, billingRepo.lastCmd)
			require.Equal(t, tc.aggregate, billingRepo.lastCmd.CacheCreationTokens)
		})
	}
}
