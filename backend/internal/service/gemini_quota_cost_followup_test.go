package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiAggregateUsageUsesAccountCost(t *testing.T) {
	totals := geminiAggregateUsage([]usagestats.ModelStat{
		{Model: "gemini-3.7-flash", Requests: 2, TotalTokens: 20, ActualCost: 5, AccountCost: 1.5},
		{Model: "gemini-4-argon", Requests: 1, TotalTokens: 10, ActualCost: 8, AccountCost: 3},
	})
	require.Equal(t, int64(2), totals.FlashRequests)
	require.Equal(t, 1.5, totals.FlashCost)
	require.Equal(t, int64(1), totals.ProRequests)
	require.Equal(t, 3.0, totals.ProCost)
}
