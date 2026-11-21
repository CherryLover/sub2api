package service

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestWeeklyQuotaRolloverRetainsSampleTimestamp(t *testing.T) {
	now := time.Now()
	expiredEnd := now.Add(-time.Minute)
	for _, keepWeekly := range []bool{false, true} {
		t.Run(fmt.Sprint(keepWeekly), func(t *testing.T) {
			sample := now.Add(-time.Hour).UTC().Format(time.RFC3339)
			extra := map[string]any{"passive_usage_sampled_at": sample, "passive_usage_7d_oi_utilization": 1.0}
			if keepWeekly {
				extra["passive_usage_7d_oi_reset"] = now.Add(24 * time.Hour).Unix()
			}
			account := &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeSetupToken, SessionWindowEnd: &expiredEnd, Extra: extra}
			headers := http.Header{}
			headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
			repo := &sessionWindowMockRepo{}
			newRateLimitServiceForTest(repo).UpdateSessionWindow(context.Background(), account, headers)
			for _, call := range repo.updateExtraCalls {
				for key, value := range call.Updates {
					extra[key] = value
				}
			}
			if keepWeekly {
				require.Equal(t, 1.0, extra["passive_usage_7d_oi_utilization"])
				require.Equal(t, sample, extra["passive_usage_sampled_at"], "retained weekly readings must keep their observation timestamp")
			} else {
				require.Nil(t, extra["passive_usage_7d_oi_utilization"])
				require.Nil(t, extra["passive_usage_sampled_at"])
			}
		})
	}
}
