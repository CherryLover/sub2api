package service

import (
	"context"
	"log/slog"
	"maps"
	"math"
	"strings"
	"time"
)

// The complete observed limit, not only its reset time, fences recovery against
// a concurrent credit restriction or a newly imposed quota limit.
type modelRateLimitSnapshotRecoveryRepository interface {
	ClearModelRateLimitSnapshotIfUnchanged(context.Context, int64, string, map[string]any) (bool, error)
}

func recoverObservedAnthropicFableLimit(ctx context.Context, repo AccountRepository, account *Account, utilization float64) bool {
	if account == nil || account.ID <= 0 || account.Platform != PlatformAnthropic ||
		(account.Type != AccountTypeOAuth && account.Type != AccountTypeSetupToken) ||
		math.IsNaN(utilization) || math.IsInf(utilization, 0) || utilization < 0 || utilization >= 0.95 {
		return false
	}
	reset := account.modelRateLimitResetAt(anthropicFableRateLimitKey)
	if reset == nil || !reset.After(time.Now()) {
		return false
	}
	oldLimits, _ := account.Extra[modelRateLimitsKey].(map[string]any)
	observed, ok := oldLimits[anthropicFableRateLimitKey].(map[string]any)
	if !ok || observed["reason"] != anthropicFableWindowReason {
		return false
	}
	conditional, ok := repo.(modelRateLimitSnapshotRecoveryRepository)
	if !ok {
		return false
	} // Never fall back to deleting all model/account limits.
	applied, err := conditional.ClearModelRateLimitSnapshotIfUnchanged(ctx, account.ID, anthropicFableRateLimitKey, maps.Clone(observed))
	if err != nil {
		slog.Warn("anthropic_fable_recovery_failed", "account_id", account.ID, "error", err)
		return false
	}
	if !applied {
		return false
	}
	limits := maps.Clone(oldLimits)
	delete(limits, anthropicFableRateLimitKey)
	extra := maps.Clone(account.Extra)
	if len(limits) == 0 {
		delete(extra, modelRateLimitsKey)
	} else {
		extra[modelRateLimitsKey] = limits
	}
	account.Extra = extra
	return true
}

func recoverObservedActiveFableUsage(ctx context.Context, repo AccountRepository, account *Account, usage *UsageInfo) bool {
	if usage == nil || usage.SevenDayFable == nil || usage.SevenDayFable.ResetsAt == nil ||
		!usage.SevenDayFable.ResetsAt.After(time.Now()) {
		return false
	}
	return recoverObservedAnthropicFableLimit(ctx, repo, account, usage.SevenDayFable.Utilization/100)
}

func validPassiveUsageUtilization(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func fableQuotaStatusRejected(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "rejected")
}
