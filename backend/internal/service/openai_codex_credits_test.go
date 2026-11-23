package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestCodexGoesOnCredits(t *testing.T) {
	cases := []struct {
		name      string
		has       bool
		unlimited bool
		balance   string
		want      bool
	}{
		{"positive balance", true, false, "1234.5", true},
		{"zero balance", true, false, "0", false},
		{"negative balance", true, false, "-1", false},
		{"no credits", false, false, "100", false},
		{"unlimited empty balance", false, true, "", true},
		{"has credits unreadable balance", true, false, "lots", true},
		{"has credits empty balance", true, false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CodexGoesOnCredits(tc.has, tc.unlimited, tc.balance); got != tc.want {
				t.Fatalf("CodexGoesOnCredits(%v,%v,%q) = %v, want %v", tc.has, tc.unlimited, tc.balance, got, tc.want)
			}
		})
	}
}

func TestBuildCodexCreditsExtraUpdates_TracksPeakBalance(t *testing.T) {
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)

	// First observation: peak equals current balance.
	u1 := BuildCodexCreditsExtraUpdates(&OpenAICredits{
		HasCredits: true,
		Unlimited:  false,
		Balance:    json.RawMessage(`"1000.5"`),
	}, nil, now)
	if u1[AccountExtraCodexCreditsBalance] != 1000.5 || u1[AccountExtraCodexCreditsPeakBalance] != 1000.5 {
		t.Fatalf("first snapshot mismatch: %+v", u1)
	}

	// Balance drops after spending: peak is preserved.
	u2 := BuildCodexCreditsExtraUpdates(&OpenAICredits{
		HasCredits: true,
		Unlimited:  false,
		Balance:    json.RawMessage(`750.25`),
	}, u1, now.Add(time.Minute))
	if u2[AccountExtraCodexCreditsBalance] != 750.25 || u2[AccountExtraCodexCreditsPeakBalance] != 1000.5 {
		t.Fatalf("spent snapshot mismatch: %+v", u2)
	}

	// Top-up raises balance above previous peak: peak updates to new high.
	u3 := BuildCodexCreditsExtraUpdates(&OpenAICredits{
		HasCredits: true,
		Unlimited:  false,
		Balance:    json.RawMessage(`"2500"`),
	}, u2, now.Add(2*time.Minute))
	if u3[AccountExtraCodexCreditsBalance] != 2500.0 || u3[AccountExtraCodexCreditsPeakBalance] != 2500.0 {
		t.Fatalf("topup snapshot mismatch: %+v", u3)
	}
}

func TestParseCodexRateLimitHeaders_Credits(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-credits-has-credits", "true")
	h.Set("x-codex-credits-unlimited", "false")
	h.Set("x-codex-credits-balance", "420.75")

	snap := ParseCodexRateLimitHeaders(h)
	if snap == nil {
		t.Fatalf("expected non-nil snapshot")
	}
	if snap.CreditsHas == nil || !*snap.CreditsHas {
		t.Fatalf("expected CreditsHas=true, got %+v", snap.CreditsHas)
	}
	if snap.CreditsUnlimited == nil || *snap.CreditsUnlimited {
		t.Fatalf("expected CreditsUnlimited=false, got %+v", snap.CreditsUnlimited)
	}
	if snap.CreditsBalance == nil || *snap.CreditsBalance != "420.75" {
		t.Fatalf("expected CreditsBalance=420.75, got %+v", snap.CreditsBalance)
	}
}

func TestShouldAutoPauseOpenAIAccountByQuota_CreditsModes(t *testing.T) {
	now := time.Now()
	ctx := withOpenAIQuotaAutoPauseSettings(context.Background(), OpsOpenAIAccountQuotaAutoPauseSettings{
		DefaultThreshold5h: 0.95,
		DefaultThreshold7d: 0.95,
	})
	baseExtra := func(mode string, hasCredits bool, balance float64) map[string]any {
		return map[string]any{
			"codex_5h_used_percent":         100.0,
			"codex_5h_reset_at":             now.Add(2 * time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at":        now.Format(time.RFC3339),
			AccountExtraCodexCreditsMode:    mode,
			AccountExtraCodexCreditsHas:     hasCredits,
			AccountExtraCodexCreditsBalance: balance,
		}
	}

	// "on" / "first" / "last" with usable credits bypass auto-pause so credits can be spent.
	for _, mode := range []string{CodexCreditsModeOn, CodexCreditsModeFirst, CodexCreditsModeLast} {
		acc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: baseExtra(mode, true, 500)}
		if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, acc); paused {
			t.Fatalf("mode %q with credits should NOT auto-pause", mode)
		}
	}

	// "off" holds the account at 100% even if auto-pause thresholds are unset.
	emptyCtx := context.Background()
	offAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: baseExtra(CodexCreditsModeOff, true, 500)}
	if paused, decision := shouldAutoPauseOpenAIAccountByQuota(emptyCtx, offAcc); !paused || decision.window != "5h" {
		t.Fatalf("mode off must hold at 100%% even without global thresholds, got paused=%v decision=%+v", paused, decision)
	}

	// Without usable credits (balance=0), "on" still auto-pauses normally.
	emptyCreditsAcc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: baseExtra(CodexCreditsModeOn, true, 0)}
	if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, emptyCreditsAcc); !paused {
		t.Fatalf("account with 0 balance should auto-pause normally")
	}
}

func TestOpenAICodexCreditsRoutingScoreAndHeadroom(t *testing.T) {
	now := time.Now()
	makeAcc := func(mode string) *Account {
		return &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_primary_used_percent":    100.0,
				"codex_7d_used_percent":         100.0,
				"codex_7d_reset_at":             now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":        now.Format(time.RFC3339),
				AccountExtraCodexCreditsMode:    mode,
				AccountExtraCodexCreditsHas:     true,
				AccountExtraCodexCreditsBalance: 300.0,
			},
		}
	}

	firstAcc := makeAcc(CodexCreditsModeFirst)
	onAcc := makeAcc(CodexCreditsModeOn)
	lastAcc := makeAcc(CodexCreditsModeLast)

	if got := openAIQuotaHeadroomFactor(firstAcc, now); got != 1.0 {
		t.Fatalf("first headroom = %v, want 1.0", got)
	}
	if got := openAIQuotaHeadroomFactor(onAcc, now); got != openAIQuotaHeadroomNeutralFactor {
		t.Fatalf("on headroom = %v, want %v", got, openAIQuotaHeadroomNeutralFactor)
	}
	if got := openAIQuotaHeadroomFactor(lastAcc, now); got != 0.0 {
		t.Fatalf("last headroom = %v, want 0.0", got)
	}

	if openAICodexCreditsScoreAdjustment(firstAcc, now) <= 0 {
		t.Fatalf("expected positive score bonus for first mode")
	}
	if openAICodexCreditsScoreAdjustment(onAcc, now) != 0 {
		t.Fatalf("expected zero score adjustment for on mode")
	}
	if openAICodexCreditsScoreAdjustment(lastAcc, now) >= 0 {
		t.Fatalf("expected negative score penalty for last mode")
	}
}
