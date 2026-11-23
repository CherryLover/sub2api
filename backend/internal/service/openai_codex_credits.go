package service

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// Codex credits routing modes (aligned with yetone/magpie).
const (
	CodexCreditsModeOn    = "on"    // Default: keep using account in place on credits when window fills.
	CodexCreditsModeFirst = "first" // Prefer spending this account's credits before routing elsewhere.
	CodexCreditsModeLast  = "last"  // Spend this account's credits only after accounts with free window headroom.
	CodexCreditsModeOff   = "off"   // Never spend credits: hold account at 100% window utilization.
)

// Account.Extra keys for OpenAI Codex credits snapshot and routing policy.
const (
	AccountExtraCodexCreditsMode           = "codex_credits_mode"
	AccountExtraCodexCreditsHas            = "codex_credits_has"
	AccountExtraCodexCreditsUnlimited      = "codex_credits_unlimited"
	AccountExtraCodexCreditsBalance        = "codex_credits_balance"
	AccountExtraCodexCreditsPeakBalance    = "codex_credits_peak_balance"
	AccountExtraCodexCreditsOverageReached = "codex_credits_overage_reached"
	AccountExtraCodexCreditsUpdatedAt      = "codex_credits_updated_at"
)

// Score adjustments applied when an account's 5h/7d window is full and it is running on credits.
const (
	openAICodexCreditsFirstBonus  = 10.0
	openAICodexCreditsLastPenalty = 10.0
)

// NormalizeCodexCreditsMode normalizes a raw mode string to one of on/first/last/off.
func NormalizeCodexCreditsMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CodexCreditsModeFirst:
		return CodexCreditsModeFirst
	case CodexCreditsModeLast:
		return CodexCreditsModeLast
	case CodexCreditsModeOff:
		return CodexCreditsModeOff
	default:
		return CodexCreditsModeOn
	}
}

// ResolveCodexCreditsMode returns the effective credits routing mode for an account.
func ResolveCodexCreditsMode(extra map[string]any) string {
	if len(extra) == 0 {
		return CodexCreditsModeOn
	}
	raw, ok := extra[AccountExtraCodexCreditsMode]
	if !ok || raw == nil {
		return CodexCreditsModeOn
	}
	s, ok := raw.(string)
	if !ok {
		return CodexCreditsModeOn
	}
	return NormalizeCodexCreditsMode(s)
}

// CodexGoesOnCredits reports whether a ChatGPT/Codex account can continue past
// a full usage window on credits (same semantics as yetone/magpie):
// unlimited => true; !has => false; balance parsed as <= 0 => false; otherwise true.
func CodexGoesOnCredits(has, unlimited bool, balance string) bool {
	if unlimited {
		return true
	}
	if !has {
		return false
	}
	trimmed := strings.TrimSpace(balance)
	if trimmed == "" {
		return true
	}
	n, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return true
	}
	return n > 0
}

// AccountHasCodexCredits reports whether account.Extra indicates usable Codex credits.
func AccountHasCodexCredits(extra map[string]any) bool {
	if len(extra) == 0 {
		return false
	}
	if resolveAccountExtraBool(extra, AccountExtraCodexCreditsUnlimited) {
		return true
	}
	if !resolveAccountExtraBool(extra, AccountExtraCodexCreditsHas) {
		return false
	}
	if bal, ok := resolveAccountExtraNumber(extra, AccountExtraCodexCreditsBalance); ok {
		return bal > 0
	}
	return true
}

// AccountCodexWindowExhausted reports whether either the 5h or 7d Codex window
// is currently at 100% utilization (and not yet reset / stale).
func AccountCodexWindowExhausted(extra map[string]any, now time.Time) bool {
	if len(extra) == 0 {
		return false
	}
	if u, ok := resolveOpenAIQuotaUtilization(extra, "5h", now); ok && u >= 1.0 {
		return true
	}
	if u, ok := resolveOpenAIQuotaUtilization(extra, "7d", now); ok && u >= 1.0 {
		return true
	}
	return false
}

// AccountIsSpendingCodexCredits reports whether the account's usage window is full
// and it is currently continuing on credits under its routing policy.
func AccountIsSpendingCodexCredits(extra map[string]any, now time.Time) bool {
	if ResolveCodexCreditsMode(extra) == CodexCreditsModeOff {
		return false
	}
	if !AccountHasCodexCredits(extra) {
		return false
	}
	return AccountCodexWindowExhausted(extra, now)
}

// openAICodexCreditsScoreAdjustment returns the additive score adjustment for
// first/last credits routing when an account is currently running on credits.
func openAICodexCreditsScoreAdjustment(account *Account, now time.Time) float64 {
	if account == nil || !account.IsOpenAIOAuth() || len(account.Extra) == 0 {
		return 0
	}
	if !AccountIsSpendingCodexCredits(account.Extra, now) {
		return 0
	}
	switch ResolveCodexCreditsMode(account.Extra) {
	case CodexCreditsModeFirst:
		return openAICodexCreditsFirstBonus
	case CodexCreditsModeLast:
		return -openAICodexCreditsLastPenalty
	default:
		return 0
	}
}

// parseRawCodexCreditsBalance parses /wham/usage credits.balance which may be
// a JSON string ("28263.2358") or a JSON number (28263.2358).
func parseRawCodexCreditsBalance(raw json.RawMessage) (float64, string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, "", false
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return 0, "", false
		}
		if n, perr := strconv.ParseFloat(asString, 64); perr == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
			return n, asString, true
		}
		return 0, asString, false
	}
	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err == nil && !math.IsNaN(asNumber) && !math.IsInf(asNumber, 0) {
		return asNumber, strconv.FormatFloat(asNumber, 'f', -1, 64), true
	}
	return 0, "", false
}

// BuildCodexCreditsExtraUpdates builds the account.Extra map updates from a
// /wham/usage credits payload, tracking peak balance for spent visualization.
func BuildCodexCreditsExtraUpdates(credits *OpenAICredits, existingExtra map[string]any, now time.Time) map[string]any {
	if credits == nil {
		return nil
	}
	updates := map[string]any{
		AccountExtraCodexCreditsHas:            credits.HasCredits,
		AccountExtraCodexCreditsUnlimited:      credits.Unlimited,
		AccountExtraCodexCreditsOverageReached: credits.OverageLimitReached,
		AccountExtraCodexCreditsUpdatedAt:      now.UTC().Format(time.RFC3339),
	}
	if bal, _, ok := parseRawCodexCreditsBalance(credits.Balance); ok {
		if bal < 0 {
			bal = 0
		}
		updates[AccountExtraCodexCreditsBalance] = bal
		peak := bal
		if prevPeak, hasPrev := resolveAccountExtraNumber(existingExtra, AccountExtraCodexCreditsPeakBalance); hasPrev && prevPeak > peak {
			peak = prevPeak
		}
		if prevBal, hasPrevBal := resolveAccountExtraNumber(existingExtra, AccountExtraCodexCreditsBalance); hasPrevBal && prevBal > peak {
			peak = prevBal
		}
		updates[AccountExtraCodexCreditsPeakBalance] = peak
	}
	return updates
}

// buildCodexCreditsHeaderUpdates builds account.Extra updates from x-codex-credits-* headers.
func buildCodexCreditsHeaderUpdates(snapshot *OpenAICodexUsageSnapshot, existingExtra map[string]any, baseTime time.Time) map[string]any {
	if snapshot == nil {
		return nil
	}
	if snapshot.CreditsHas == nil && snapshot.CreditsUnlimited == nil && snapshot.CreditsBalance == nil {
		return nil
	}
	updates := map[string]any{
		AccountExtraCodexCreditsUpdatedAt: baseTime.UTC().Format(time.RFC3339),
	}
	if snapshot.CreditsHas != nil {
		updates[AccountExtraCodexCreditsHas] = *snapshot.CreditsHas
	}
	if snapshot.CreditsUnlimited != nil {
		updates[AccountExtraCodexCreditsUnlimited] = *snapshot.CreditsUnlimited
	}
	if snapshot.CreditsBalance != nil {
		trimmed := strings.TrimSpace(*snapshot.CreditsBalance)
		if n, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
			if n < 0 {
				n = 0
			}
			updates[AccountExtraCodexCreditsBalance] = n
			peak := n
			if prevPeak, hasPrev := resolveAccountExtraNumber(existingExtra, AccountExtraCodexCreditsPeakBalance); hasPrev && prevPeak > peak {
				peak = prevPeak
			}
			if prevBal, hasPrevBal := resolveAccountExtraNumber(existingExtra, AccountExtraCodexCreditsBalance); hasPrevBal && prevBal > peak {
				peak = prevBal
			}
			updates[AccountExtraCodexCreditsPeakBalance] = peak
		}
	}
	return updates
}
