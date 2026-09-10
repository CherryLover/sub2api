//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func schedulingDiagnosisSources(d AccountSchedulingDiagnosis) []string {
	sources := make([]string, 0, len(d.Blocks))
	for _, block := range d.Blocks {
		sources = append(sources, block.Source)
	}
	return sources
}

func newSchedulingDiagnosisAccount(id int64, accountType string) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        accountType,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func TestDiagnoseAccountScheduling_NilGatewayReportsPersistedBlocksOnly(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	healthy := newSchedulingDiagnosisAccount(1, AccountTypeOAuth)

	blocked := newSchedulingDiagnosisAccount(2, AccountTypeOAuth)
	blocked.Status = "error"
	blocked.Schedulable = false
	blocked.AutoPauseOnExpired = true
	blocked.ExpiresAt = &past
	blocked.OverloadUntil = &future
	blocked.RateLimitResetAt = &future
	blocked.TempUnschedulableUntil = &future
	blocked.TempUnschedulableReason = "token refresh failed"

	elapsed := newSchedulingDiagnosisAccount(3, AccountTypeOAuth)
	elapsed.ExpiresAt = &past // AutoPauseOnExpired off
	elapsed.OverloadUntil = &past
	elapsed.RateLimitResetAt = &past
	elapsed.TempUnschedulableUntil = &past

	quotaExceeded := newSchedulingDiagnosisAccount(4, AccountTypeAPIKey)
	quotaExceeded.Extra = map[string]any{"quota_limit": 10.0, "quota_used": 12.0}

	// Quota auto-pause is gateway-level; without a gateway it is not reported.
	autoPaused := newSchedulingDiagnosisAccount(5, AccountTypeOAuth)
	autoPaused.Extra = map[string]any{
		"auto_pause_7d_threshold": 0.9,
		"codex_7d_used_percent":   99.0,
	}

	var gateway *OpenAIGatewayService
	got := gateway.DiagnoseAccountScheduling(context.Background(), []*Account{healthy, blocked, elapsed, quotaExceeded, autoPaused, nil})
	require.Len(t, got, 5)

	require.True(t, got[1].Schedulable)
	require.NotNil(t, got[1].Blocks, "blocks must serialize as [] rather than null")
	require.Empty(t, got[1].Blocks)

	d2 := got[2]
	require.False(t, d2.Schedulable)
	require.Equal(t, []string{
		AccountSchedulingBlockStatus,
		AccountSchedulingBlockManualUnschedulable,
		AccountSchedulingBlockExpired,
		AccountSchedulingBlockOverloaded,
		AccountSchedulingBlockRateLimited,
		AccountSchedulingBlockTempUnschedulable,
	}, schedulingDiagnosisSources(d2))
	require.Equal(t, "error", d2.Blocks[0].Status)
	require.Nil(t, d2.Blocks[1].Until)
	require.Nil(t, d2.Blocks[2].Until, "expiry has no future deadline")
	for _, block := range d2.Blocks[3:] {
		require.NotNil(t, block.Until, block.Source)
		require.True(t, future.Equal(*block.Until), block.Source)
		require.Equal(t, time.UTC, block.Until.Location(), block.Source)
	}
	require.Equal(t, "token refresh failed", d2.Blocks[5].Reason)

	require.True(t, got[3].Schedulable, "elapsed deadlines are not blocks")
	require.Equal(t, []string{AccountSchedulingBlockQuotaExceeded}, schedulingDiagnosisSources(got[4]))
	require.True(t, got[5].Schedulable)

	for _, account := range []*Account{healthy, blocked, elapsed, quotaExceeded} {
		require.Equal(t, account.IsSchedulable(), got[account.ID].Schedulable, "account %d must agree with IsSchedulable", account.ID)
	}
}

func TestDiagnoseAccountScheduling_ReportsFutureRuntimeBlock(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newSchedulingDiagnosisAccount(10, AccountTypeOAuth)
	until := time.Now().Add(96 * time.Hour)
	svc.BlockAccountScheduling(account, until, "429")

	d := svc.DiagnoseAccountScheduling(context.Background(), []*Account{account})[10]
	require.False(t, d.Schedulable)
	require.Equal(t, []string{AccountSchedulingBlockRuntime}, schedulingDiagnosisSources(d))
	require.NotNil(t, d.Blocks[0].Until)
	require.True(t, until.Equal(*d.Blocks[0].Until))

	// Diagnosis leaves the block in place for the scheduler.
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestDiagnoseAccountScheduling_ExpiredRuntimeBlockIsNotReportedNorDeleted(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newSchedulingDiagnosisAccount(11, AccountTypeOAuth)
	expired := time.Now().Add(-time.Minute)
	svc.openaiAccountRuntimeBlockUntil.Store(account.ID, expired)

	// Non-OpenAI accounts are outside the gateway's runtime block even when
	// an entry exists for their ID.
	anthropic := &Account{ID: 12, Platform: "anthropic", Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	svc.openaiAccountRuntimeBlockUntil.Store(anthropic.ID, time.Now().Add(time.Hour))

	got := svc.DiagnoseAccountScheduling(context.Background(), []*Account{account, anthropic})
	require.True(t, got[11].Schedulable)
	require.Empty(t, got[11].Blocks)
	require.True(t, got[12].Schedulable)

	stored, ok := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.True(t, ok, "diagnosis must not delete the expired entry")
	storedUntil, isTime := stored.(time.Time)
	require.True(t, isTime)
	require.True(t, expired.Equal(storedUntil))
	_, bumped := svc.openaiAccountRuntimeBlockGeneration.Load(account.ID)
	require.False(t, bumped, "diagnosis must not bump the block generation")
	require.Zero(t, svc.openaiAccountRuntimeBlockSequence.Load())
	_, locked := svc.openaiAccountRuntimeBlockLocks.Load(account.ID)
	require.False(t, locked, "diagnosis must not allocate a per-account lock")
}

func TestDiagnoseAccountScheduling_ListsActiveModelBlocksWithoutTouchingState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newSchedulingDiagnosisAccount(20, AccountTypeAPIKey)
	now := time.Now()

	state := svc.getOpenAIAccountModelTransientState()
	state.recordFailure(20, "gpt-6-astra", now)
	decision := state.recordFailure(20, "GPT-6-Astra", now) // streak 2 -> cooldown
	require.False(t, decision.BlockUntil.IsZero())
	state.recordFailure(20, "gpt-5-once", now) // streak 1 -> no cooldown
	state.recordFailure(21, "gpt-other", now)
	state.recordFailure(21, "gpt-other", now) // another account's cooldown
	// A cooldown whose streak is past the TTL is stale and must not be listed,
	// but diagnosis must not clean it up either.
	staleKey := openAIAccountModelKey{AccountID: 20, Model: "gpt-stale"}
	state.entries[staleKey] = openAIAccountModelTransientEntry{
		failureStreak: 3,
		lastFailure:   now.Add(-openAIModelTransientStreakTTL - time.Minute),
		blockUntil:    now.Add(time.Minute),
		lastTouched:   now.Add(-time.Hour),
	}

	before := make(map[openAIAccountModelKey]openAIAccountModelTransientEntry, len(state.entries))
	for key, entry := range state.entries {
		before[key] = entry
	}

	d := svc.DiagnoseAccountScheduling(context.Background(), []*Account{account})[20]
	require.True(t, d.Schedulable, "a model-scoped cooldown does not exclude the account for other models")
	require.Equal(t, []string{AccountSchedulingBlockModelRuntime}, schedulingDiagnosisSources(d))
	require.Equal(t, "gpt-6-astra", d.Blocks[0].Model)
	require.NotNil(t, d.Blocks[0].Until)
	require.True(t, decision.BlockUntil.Equal(*d.Blocks[0].Until))

	require.Equal(t, before, state.entries, "diagnosis must not delete entries or refresh lastTouched")
}

func TestDiagnoseAccountScheduling_ReportsProxyQuarantineWithoutExpiringEntries(t *testing.T) {
	svc := &OpenAIGatewayService{}
	activeProxy := int64(3)
	expiredProxy := int64(4)
	quarantined := newSchedulingDiagnosisAccount(30, AccountTypeOAuth)
	quarantined.ProxyID = &activeProxy
	recovered := newSchedulingDiagnosisAccount(31, AccountTypeOAuth)
	recovered.ProxyID = &expiredProxy

	now := time.Now()
	blockedUntil := now.Add(5 * time.Minute)
	circuit := svc.getOpenAIProxyStreamCircuit()
	circuit.entries[activeProxy] = openAIProxyStreamCircuitEntry{failureCount: 2, blockedUntil: blockedUntil, lastTouched: now}
	circuit.entries[expiredProxy] = openAIProxyStreamCircuitEntry{failureCount: 2, blockedUntil: now.Add(-time.Second), lastTouched: now}

	got := svc.DiagnoseAccountScheduling(context.Background(), []*Account{quarantined, recovered})

	d := got[30]
	require.False(t, d.Schedulable)
	require.Equal(t, []string{AccountSchedulingBlockProxyQuarantine}, schedulingDiagnosisSources(d))
	require.Equal(t, activeProxy, d.Blocks[0].ProxyID)
	require.NotNil(t, d.Blocks[0].Until)
	require.True(t, blockedUntil.Equal(*d.Blocks[0].Until))

	require.True(t, got[31].Schedulable)
	_, kept := circuit.entries[expiredProxy]
	require.True(t, kept, "diagnosis must not expire circuit entries")
}

func TestDiagnoseAccountScheduling_ReportsQuotaAutoPause(t *testing.T) {
	svc := &OpenAIGatewayService{}
	updatedAt := time.Now().UTC().Format(time.RFC3339)

	paused := newSchedulingDiagnosisAccount(40, AccountTypeOAuth)
	paused.Extra = map[string]any{
		"auto_pause_7d_threshold": 0.9,
		"codex_7d_used_percent":   93.0,
		"codex_usage_updated_at":  updatedAt,
	}
	below := newSchedulingDiagnosisAccount(41, AccountTypeOAuth)
	below.Extra = map[string]any{
		"auto_pause_7d_threshold": 0.9,
		"codex_7d_used_percent":   80.0,
		"codex_usage_updated_at":  updatedAt,
	}

	got := svc.DiagnoseAccountScheduling(context.Background(), []*Account{paused, below})

	d := got[40]
	require.False(t, d.Schedulable)
	require.Equal(t, []string{AccountSchedulingBlockQuotaAutoPause}, schedulingDiagnosisSources(d))
	require.Equal(t, "7d", d.Blocks[0].Window)
	require.InDelta(t, 0.9, d.Blocks[0].Threshold, 1e-9)
	require.InDelta(t, 0.93, d.Blocks[0].Utilization, 1e-9)
	require.Nil(t, d.Blocks[0].Until)

	require.True(t, got[41].Schedulable)
}
