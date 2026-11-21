package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAntigravityQuotaRetainsNonzeroFractionalUsage(t *testing.T) {
	for _, tc := range []struct {
		name            string
		remaining, used float64
	}{
		{"small", 0.99978864, 0.021136}, {"fraction", 0.98016745, 1.983255}, {"tiny", 0.99999999, 0.000001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := (&AntigravityQuotaFetcher{}).buildUsageInfo(&antigravity.FetchAvailableModelsResponse{Models: map[string]antigravity.ModelInfo{
				"claude-sonnet-4-20250514": {QuotaInfo: &antigravity.ModelQuotaInfo{RemainingFraction: tc.remaining}},
			}}, "", "", nil)
			quota := info.AntigravityQuota["claude-sonnet-4-20250514"]
			require.NotNil(t, quota)
			require.InDelta(t, tc.used, quota.Utilization, 1e-8)
			require.InDelta(t, info.FiveHour.Utilization, quota.Utilization, 1e-8, "both presentations must use the same observation")
		})
	}
}
