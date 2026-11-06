package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type keyUsageDashboardRepoStub struct {
	hourRows               []KeyUsageDashboardHourRow
	hourBucketCalls        int
	comparisonScope        KeyUsageDashboardScope
	comparisonTodayStart   time.Time
	comparisonTodayEnd     time.Time
	comparisonPreviousFrom time.Time
}

func (s *keyUsageDashboardRepoStub) GetUsageTotals(context.Context, KeyUsageDashboardScope) (KeyUsageDashboardAggregate, error) {
	return KeyUsageDashboardAggregate{}, nil
}

func (s *keyUsageDashboardRepoStub) GetKeyHourBuckets(context.Context, KeyUsageDashboardScope) ([]KeyUsageDashboardHourRow, error) {
	s.hourBucketCalls++
	return s.hourRows, nil
}

func (s *keyUsageDashboardRepoStub) GetPeakRPM(context.Context, KeyUsageDashboardScope) ([]KeyUsageDashboardPeakRow, error) {
	return nil, nil
}

func (s *keyUsageDashboardRepoStub) GetErrorAggregates(context.Context, KeyUsageDashboardScope, KeyUsageDashboardScope) ([]KeyUsageDashboardErrorRow, error) {
	return nil, nil
}

func (s *keyUsageDashboardRepoStub) GetModelAggregates(context.Context, KeyUsageDashboardScope) ([]KeyUsageDashboardModelRow, error) {
	return nil, nil
}

func (s *keyUsageDashboardRepoStub) GetEffortAggregates(context.Context, KeyUsageDashboardScope) ([]KeyUsageDashboardEffortRow, error) {
	return nil, nil
}

func (s *keyUsageDashboardRepoStub) GetKeyCostComparisons(
	_ context.Context,
	scope KeyUsageDashboardScope,
	todayStart time.Time,
	todayEnd time.Time,
	previousStart time.Time,
) ([]KeyUsageDashboardCostComparisonRow, error) {
	s.comparisonScope = scope
	s.comparisonTodayStart = todayStart
	s.comparisonTodayEnd = todayEnd
	s.comparisonPreviousFrom = previousStart
	return nil, nil
}

type keyUsageDashboardAPIKeyStub struct {
	keys []APIKey
}

func (s *keyUsageDashboardAPIKeyStub) ListAllByUserID(context.Context, int64) ([]APIKey, error) {
	return s.keys, nil
}

func TestResolveKeyUsageDashboardWindow(t *testing.T) {
	loc := keyUsageDashboardLocation
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, loc)

	today, err := ResolveKeyUsageDashboardWindow("today", now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 11, 6, 0, 0, 0, 0, loc), today.Start)
	require.Equal(t, now, today.End)
	require.Equal(t, time.Date(2026, 11, 5, 0, 0, 0, 0, loc), today.PreviousStart)
	require.Equal(t, time.Date(2026, 11, 5, 15, 30, 0, 0, loc), today.PreviousEnd)
	require.Len(t, today.SeriesLabels, 24)
	require.Equal(t, "0:00", today.SeriesLabels[0])
	require.Equal(t, "23:00", today.SeriesLabels[23])

	week, err := ResolveKeyUsageDashboardWindow("7d", now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 31, 0, 0, 0, 0, loc), week.Start)
	require.Equal(t, time.Date(2026, 10, 24, 0, 0, 0, 0, loc), week.PreviousStart)
	require.Equal(t, time.Date(2026, 10, 30, 15, 30, 0, 0, loc), week.PreviousEnd)
	require.Len(t, week.SeriesLabels, 7)

	month, err := ResolveKeyUsageDashboardWindow("30d", now)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 8, 0, 0, 0, 0, loc), month.Start)
	require.Len(t, month.SeriesLabels, 30)

	_, err = ResolveKeyUsageDashboardWindow("90d", now)
	require.ErrorIs(t, err, ErrInvalidKeyUsageDashboardRange)
}

func TestKeyUsageDashboardSeriesFillsMissingBuckets(t *testing.T) {
	loc := keyUsageDashboardLocation
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, loc)
	today, err := ResolveKeyUsageDashboardWindow("today", now)
	require.NoError(t, err)
	rows := []KeyUsageDashboardHourRow{
		{
			APIKeyID: 7,
			Date:     "2026-11-06",
			Hour:     9,
			KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{
				Requests: 2,
				Cost:     0.5,
			},
		},
	}
	series := keyUsageDashboardSeries(keyUsageDashboardSeriesByKey(rows, today)[7], today)
	require.Len(t, series, 24)
	require.Equal(t, int64(2), series[9].Requests)
	require.InDelta(t, 0.5, series[9].Cost, 0.000001)
	require.Zero(t, series[0].Requests)
	require.Zero(t, series[23].Requests)

	week, err := ResolveKeyUsageDashboardWindow("7d", now)
	require.NoError(t, err)
	weekRows := []KeyUsageDashboardHourRow{
		{APIKeyID: 7, Date: "2026-11-01", Hour: 11, KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 3}},
	}
	weekSeries := keyUsageDashboardSeries(keyUsageDashboardSeriesByKey(weekRows, week)[7], week)
	require.Len(t, weekSeries, 7)
	require.Equal(t, int64(3), weekSeries[1].Requests)
	require.Zero(t, weekSeries[0].Requests)
	require.Zero(t, weekSeries[6].Requests)
}

func TestBuildKeyUsageDashboardAlertsBoundariesAndOrder(t *testing.T) {
	loc := keyUsageDashboardLocation
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, loc)
	recent := now.AddDate(0, 0, -1)
	apiKeys := []APIKey{
		{ID: 1, Name: "error-and-idle", Status: StatusAPIKeyActive},
		{ID: 2, Name: "recent", Status: StatusAPIKeyActive, LastUsedAt: &recent},
		{ID: 3, Name: "below-threshold", Status: StatusAPIKeyActive, LastUsedAt: &recent},
	}
	totals := map[int64]KeyUsageDashboardAggregate{
		1: {Requests: 380},
		2: {Requests: 400},
		3: {Requests: 400},
	}
	errorsByKey := map[int64]KeyUsageDashboardErrorRow{
		1: {APIKeyID: 1, ErrorCount: 20, LargeRequestCount: 5, LargeRequestAvgBytes: 5 * 1024 * 1024},
		2: {APIKeyID: 2, ErrorCount: 19, LargeRequestCount: 4, LargeRequestAvgBytes: 6 * 1024 * 1024},
		3: {APIKeyID: 3, ErrorCount: 19, LargeRequestCount: 4},
	}
	comparisons := []KeyUsageDashboardCostComparisonRow{
		{APIKeyID: 1, TodayCost: 5, PreviousCost: 70},
		{APIKeyID: 2, TodayCost: 5, PreviousCost: 0},
		{APIKeyID: 3, TodayCost: 4.99, PreviousCost: 0},
	}

	alerts := buildKeyUsageDashboardAlerts(
		apiKeys,
		totals,
		errorsByKey,
		comparisons,
		now,
		KeyUsageDashboardFilters{},
	)

	require.Len(t, alerts, 4)
	require.Equal(t, "high_error_rate", alerts[0].Type)
	require.Equal(t, int64(1), alerts[0].APIKeyID)
	require.InDelta(t, 0.05, alerts[0].Value, 0.000001)
	require.Equal(t, "large_request", alerts[1].Type)
	require.Equal(t, int64(1), alerts[1].APIKeyID)
	require.Equal(t, int64(5), alerts[1].Count)
	require.Equal(t, "usage_spike", alerts[2].Type)
	require.Equal(t, int64(2), alerts[2].APIKeyID)
	require.Zero(t, alerts[2].Count)
	require.Equal(t, "idle", alerts[3].Type)
	require.Equal(t, int64(1), alerts[3].APIKeyID)
	require.Equal(t, int64(1), alerts[3].Count)

	filtered := buildKeyUsageDashboardAlerts(
		apiKeys,
		totals,
		errorsByKey,
		comparisons,
		now,
		KeyUsageDashboardFilters{Model: "gpt-5"},
	)
	for index := range filtered {
		require.NotEqual(t, "idle", filtered[index].Type)
	}
}

func TestBuildKeyUsageDashboardAlertsIncludeErrorOnlyKeys(t *testing.T) {
	apiKeys := []APIKey{
		{ID: 4, Name: "errors-only", Status: StatusAPIKeyInactive},
	}
	errorsByKey := map[int64]KeyUsageDashboardErrorRow{
		4: {
			APIKeyID:             4,
			ErrorCount:           20,
			LargeRequestCount:    5,
			LargeRequestAvgBytes: 5 * 1024 * 1024,
		},
	}

	alerts := buildKeyUsageDashboardAlerts(
		apiKeys,
		nil,
		errorsByKey,
		nil,
		time.Date(2026, 11, 6, 15, 30, 0, 0, keyUsageDashboardLocation),
		KeyUsageDashboardFilters{},
	)

	require.Len(t, alerts, 2)
	require.Equal(t, "high_error_rate", alerts[0].Type)
	require.Equal(t, int64(4), alerts[0].APIKeyID)
	require.Equal(t, int64(20), alerts[0].Count)
	require.Equal(t, "large_request", alerts[1].Type)
	require.Equal(t, int64(4), alerts[1].APIKeyID)
	require.Equal(t, int64(5), alerts[1].Count)
}

func TestBuildKeyUsageDashboardAlertsExcludeRecentlyCreatedIdleKeys(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, keyUsageDashboardLocation)
	recentlyCreated := now.AddDate(0, 0, -3)
	longAgoCreated := now.AddDate(0, 0, -30)
	lastUsedLongAgo := now.AddDate(0, 0, -20)
	apiKeys := []APIKey{
		{ID: 1, Name: "new-unused", Status: StatusAPIKeyActive, CreatedAt: recentlyCreated},
		{ID: 2, Name: "old-unused-since", Status: StatusAPIKeyActive, CreatedAt: longAgoCreated, LastUsedAt: &lastUsedLongAgo},
	}

	alerts := buildKeyUsageDashboardAlerts(
		apiKeys,
		nil,
		nil,
		nil,
		now,
		KeyUsageDashboardFilters{},
	)

	require.Len(t, alerts, 1)
	require.Equal(t, "idle", alerts[0].Type)
	require.Equal(t, int64(2), alerts[0].APIKeyID)
	require.Equal(t, int64(1), alerts[0].Count)
}

func TestBuildKeyUsageDashboardUsageSpikeBoundaries(t *testing.T) {
	apiKeys := []APIKey{
		{ID: 1, Name: "two-times", Status: StatusAPIKeyInactive},
		{ID: 2, Name: "below-ratio", Status: StatusAPIKeyInactive},
		{ID: 3, Name: "below-minimum", Status: StatusAPIKeyInactive},
		{ID: 4, Name: "no-history", Status: StatusAPIKeyInactive},
	}
	comparisons := []KeyUsageDashboardCostComparisonRow{
		{APIKeyID: 1, TodayCost: 20, PreviousCost: 70},
		{APIKeyID: 2, TodayCost: 19.99, PreviousCost: 70},
		{APIKeyID: 3, TodayCost: 4.99, PreviousCost: 0},
		{APIKeyID: 4, TodayCost: 5, PreviousCost: 0},
	}

	alerts := buildKeyUsageDashboardAlerts(
		apiKeys,
		nil,
		nil,
		comparisons,
		time.Date(2026, 11, 6, 15, 30, 0, 0, keyUsageDashboardLocation),
		KeyUsageDashboardFilters{},
	)

	require.Len(t, alerts, 2)
	require.Equal(t, int64(4), alerts[0].APIKeyID)
	require.InDelta(t, 5, alerts[0].Value, 0.000001)
	require.Equal(t, int64(1), alerts[1].APIKeyID)
	require.InDelta(t, 2, alerts[1].Value, 0.000001)
}

func TestKeyUsageDashboardUsageSpikeUsesTodayForAllRanges(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, keyUsageDashboardLocation)
	repo := &keyUsageDashboardRepoStub{}
	keys := &keyUsageDashboardAPIKeyStub{
		keys: []APIKey{{ID: 7, Name: "key", Status: StatusAPIKeyActive, LastUsedAt: &now}},
	}
	service := NewKeyUsageDashboardService(repo, keys)
	service.now = func() time.Time { return now }

	_, err := service.GetDashboard(context.Background(), 42, "30d", KeyUsageDashboardFilters{})
	require.NoError(t, err)

	todayStart := time.Date(2026, 11, 6, 0, 0, 0, 0, keyUsageDashboardLocation)
	require.Equal(t, todayStart, repo.comparisonTodayStart)
	require.Equal(t, now, repo.comparisonTodayEnd)
	require.Equal(t, todayStart.AddDate(0, 0, -7), repo.comparisonPreviousFrom)
	require.Equal(t, int64(42), repo.comparisonScope.UserID)
}

func TestBuildKeyUsageDashboardDetailModelsAndEfforts(t *testing.T) {
	modelRows := []KeyUsageDashboardModelRow{
		{Model: "m1", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 6}},
		{Model: "m2", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 5}},
		{Model: "m3", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 4}},
		{Model: "m4", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 3}},
		{Model: "m5", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 2}},
		{Model: "m6", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 1}},
		{Model: "m7", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Cost: 7}},
	}
	models := buildKeyUsageDashboardDetailModels(modelRows)
	require.Len(t, models, 6)
	require.Equal(t, "m7", models[0].Model)
	require.Equal(t, "m4", models[4].Model)
	require.Equal(t, "其他", models[5].Model)
	require.InDelta(t, 3, models[5].Metrics.Cost, 0.000001)

	efforts := buildKeyUsageDashboardEfforts([]KeyUsageDashboardEffortRow{
		{Effort: "unknown", Requests: 100},
		{Effort: "low", Requests: 2},
		{Effort: "high", Requests: 5},
	})
	require.Equal(t, []string{"high", "low", "unknown"}, []string{
		efforts[0].Effort,
		efforts[1].Effort,
		efforts[2].Effort,
	})
}

func TestBuildKeyUsageDashboardModelSummaryKeepsCalledKeysWithoutTopModels(t *testing.T) {
	keys := []KeyUsageDashboardKey{
		{APIKeyID: 1, Totals: KeyUsageDashboardMetric{Requests: 5}},
		{APIKeyID: 2, Totals: KeyUsageDashboardMetric{Requests: 1}},
		{APIKeyID: 3},
	}
	rows := []KeyUsageDashboardModelRow{
		{APIKeyID: 1, Model: "m1", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 1, Cost: 5}},
		{APIKeyID: 1, Model: "m2", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 1, Cost: 4}},
		{APIKeyID: 1, Model: "m3", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 1, Cost: 3}},
		{APIKeyID: 1, Model: "m4", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 1, Cost: 2}},
		{APIKeyID: 1, Model: "m5", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 1, Cost: 1}},
		{APIKeyID: 2, Model: "m6", KeyUsageDashboardAggregate: KeyUsageDashboardAggregate{Requests: 1, Cost: 0.5}},
	}

	summary := buildKeyUsageDashboardModelSummary(rows, keys)
	require.Equal(t, []string{"m1", "m2", "m3", "m4", "m5"}, summary.Names)
	require.Len(t, summary.Keys, 2)
	require.Equal(t, int64(2), summary.Keys[1].APIKeyID)
	require.Len(t, summary.Keys[1].Cells, 5)
	for index := range summary.Keys[1].Cells {
		require.Zero(t, summary.Keys[1].Cells[index].Requests)
	}
}

func TestKeyUsageDashboardCacheTTL(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, keyUsageDashboardLocation)
	repo := &keyUsageDashboardRepoStub{}
	keys := &keyUsageDashboardAPIKeyStub{
		keys: []APIKey{{ID: 7, Name: "key", Status: StatusAPIKeyActive}},
	}
	service := NewKeyUsageDashboardService(repo, keys)
	service.now = func() time.Time { return now }

	_, err := service.GetDashboard(context.Background(), 42, "7d", KeyUsageDashboardFilters{})
	require.NoError(t, err)
	_, err = service.GetDashboard(context.Background(), 42, "7d", KeyUsageDashboardFilters{})
	require.NoError(t, err)
	require.Equal(t, 1, repo.hourBucketCalls)

	now = now.Add(KeyUsageDashboardCacheTTL + time.Second)
	_, err = service.GetDashboard(context.Background(), 42, "7d", KeyUsageDashboardFilters{})
	require.NoError(t, err)
	require.Equal(t, 2, repo.hourBucketCalls)
}

func TestKeyUsageDashboardCacheKeysFiltersAndCapsEntries(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 30, 0, 0, keyUsageDashboardLocation)
	repo := &keyUsageDashboardRepoStub{}
	keys := &keyUsageDashboardAPIKeyStub{
		keys: []APIKey{{ID: 7, Name: "key", Status: StatusAPIKeyActive}},
	}
	service := NewKeyUsageDashboardService(repo, keys)
	service.now = func() time.Time { return now }
	service.maxEntries = 2

	for _, filters := range []KeyUsageDashboardFilters{
		{},
		{},
		{GroupID: 12},
		{Model: "gpt-5"},
	} {
		_, err := service.GetDashboard(context.Background(), 42, "7d", filters)
		require.NoError(t, err)
	}

	require.Equal(t, 3, repo.hourBucketCalls)
	service.cacheMu.RLock()
	cacheEntries := len(service.cache)
	service.cacheMu.RUnlock()
	require.Equal(t, 2, cacheEntries)
}
