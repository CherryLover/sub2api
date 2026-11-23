package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type stubUsageRepoForAPIKeyModelStats struct {
	UsageLogRepository
	gotStart   time.Time
	gotEnd     time.Time
	gotFilters usagestats.UsageLogFilters
	gotSource  string
	stats      []usagestats.ModelStat
}

func (r *stubUsageRepoForAPIKeyModelStats) GetModelStatsWithUsageFiltersBySource(
	_ context.Context,
	startTime, endTime time.Time,
	filters usagestats.UsageLogFilters,
	source string,
) ([]usagestats.ModelStat, error) {
	r.gotStart = startTime
	r.gotEnd = endTime
	r.gotFilters = filters
	r.gotSource = source
	return r.stats, nil
}

func TestUsageService_GetAPIKeyModelStats_UsesUpstreamModelSource(t *testing.T) {
	start := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	repo := &stubUsageRepoForAPIKeyModelStats{
		stats: []usagestats.ModelStat{
			{Model: "deepseek-flash", Requests: 12, TotalTokens: 8000, ActualCost: 0.25},
		},
	}
	svc := NewUsageService(repo, nil, nil, nil)

	got, err := svc.GetAPIKeyModelStats(context.Background(), 21, start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotSource != usagestats.ModelSourceUpstream {
		t.Fatalf("expected model source %q, got %q", usagestats.ModelSourceUpstream, repo.gotSource)
	}
	if repo.gotFilters.APIKeyID != 21 {
		t.Fatalf("expected api_key_id=21, got %d", repo.gotFilters.APIKeyID)
	}
	if !repo.gotStart.Equal(start) || !repo.gotEnd.Equal(end) {
		t.Fatalf("unexpected time range: [%v, %v)", repo.gotStart, repo.gotEnd)
	}
	if len(got) != 1 || got[0].Model != "deepseek-flash" {
		t.Fatalf("unexpected stats: %+v", got)
	}
}
