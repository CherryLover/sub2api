//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type kimiConcurrencyRateLimitRepo struct {
	rateLimitAccountRepoStub
	rateLimitedCalls int
	lastResetAt time.Time
	rateLimitErr error
}

func (r *kimiConcurrencyRateLimitRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitedCalls++
	r.lastResetAt = resetAt
	return r.rateLimitErr
}

func TestKimiConcurrencyFallbackHonorsSecondsAndDisabledSetting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		seconds int
	}{
		{"custom short cooldown", true, 17},
		{"cooldown disabled", false, 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &kimiConcurrencyRateLimitRepo{}
			blocker := &runtimeBlockRecorder{}
			cfg := &config.Config{}
			svc := NewRateLimitService(repo, nil, cfg, nil, nil)
			svc.SetAccountRuntimeBlocker(blocker)
			settingRepo := newMockSettingRepo()
			payload, err := json.Marshal(RateLimit429CooldownSettings{Enabled: tc.enabled, CooldownSeconds: tc.seconds})
			require.NoError(t, err)
			settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(payload)
			svc.SetSettingService(NewSettingService(settingRepo, cfg))
			before := time.Now()
			svc.handleCNProviderConcurrencyLimit403(context.Background(), &Account{ID: 4, Platform: PlatformKimi})
			if tc.enabled {
				require.Equal(t, 1, repo.rateLimitedCalls)
				require.WithinDuration(t, before.Add(time.Duration(tc.seconds)*time.Second), repo.lastResetAt, time.Second)
				require.Equal(t, []string{"429_fallback"}, blocker.reasons)
			} else {
				require.Zero(t, repo.rateLimitedCalls)
				require.Empty(t, blocker.reasons)
			}
			require.Zero(t, repo.tempCalls)
		})
	}
}
