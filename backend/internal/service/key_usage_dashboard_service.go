package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	keyUsageDashboardLocationName = "Asia/Shanghai"
	keyUsageDashboardModelLimit   = 5
	keyUsageDashboardAlertLimit   = 3
)

var (
	// ErrInvalidKeyUsageDashboardRange indicates an unsupported dashboard range.
	ErrInvalidKeyUsageDashboardRange = errors.New("invalid key usage dashboard range")
	keyUsageDashboardLocation        = mustLoadKeyUsageDashboardLocation()
)

type keyUsageDashboardAPIKeyLister interface {
	ListAllByUserID(ctx context.Context, userID int64) ([]APIKey, error)
}

type keyUsageDashboardCacheEntry struct {
	response  *KeyUsageDashboardResponse
	expiresAt time.Time
}

// KeyUsageDashboardService builds the user's API key usage dashboard.
type KeyUsageDashboardService struct {
	repo       KeyUsageDashboardRepository
	apiKeys    keyUsageDashboardAPIKeyLister
	cacheTTL   time.Duration
	maxEntries int
	now        func() time.Time

	cacheMu sync.RWMutex
	cache   map[string]keyUsageDashboardCacheEntry
}

// NewKeyUsageDashboardService creates a cached key usage dashboard service.
func NewKeyUsageDashboardService(repo KeyUsageDashboardRepository, apiKeys keyUsageDashboardAPIKeyLister) *KeyUsageDashboardService {
	return &KeyUsageDashboardService{
		repo:       repo,
		apiKeys:    apiKeys,
		cacheTTL:   KeyUsageDashboardCacheTTL,
		maxEntries: KeyUsageDashboardCacheMaxEntries,
		now:        time.Now,
		cache:      make(map[string]keyUsageDashboardCacheEntry),
	}
}

func mustLoadKeyUsageDashboardLocation() *time.Location {
	loc, err := time.LoadLocation(keyUsageDashboardLocationName)
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

// ResolveKeyUsageDashboardWindow validates range and computes current/previous windows.
func ResolveKeyUsageDashboardWindow(rangeName string, now time.Time) (KeyUsageDashboardWindow, error) {
	rangeName = strings.TrimSpace(rangeName)
	days := 0
	switch rangeName {
	case KeyUsageDashboardRangeToday:
		days = 1
	case KeyUsageDashboardRange7d:
		days = 7
	case KeyUsageDashboardRange30d:
		days = 30
	default:
		return KeyUsageDashboardWindow{}, fmt.Errorf("%w: %q", ErrInvalidKeyUsageDashboardRange, rangeName)
	}

	localNow := now.In(keyUsageDashboardLocation)
	start := startOfKeyUsageDashboardDay(localNow)
	if days > 1 {
		start = start.AddDate(0, 0, -(days - 1))
	}

	window := KeyUsageDashboardWindow{
		Range:         rangeName,
		Start:         start,
		End:           localNow,
		PreviousStart: start.AddDate(0, 0, -days),
		PreviousEnd:   localNow.AddDate(0, 0, -days),
		Days:          days,
	}
	if rangeName == KeyUsageDashboardRangeToday {
		window.SeriesLabels = make([]string, 24)
		for hour := 0; hour < 24; hour++ {
			window.SeriesLabels[hour] = fmt.Sprintf("%d:00", hour)
		}
		return window, nil
	}

	for day := startOfKeyUsageDashboardDay(start); !day.After(startOfKeyUsageDashboardDay(localNow)); day = day.AddDate(0, 0, 1) {
		window.SeriesLabels = append(window.SeriesLabels, day.Format("01/02"))
	}
	return window, nil
}

func startOfKeyUsageDashboardDay(value time.Time) time.Time {
	value = value.In(keyUsageDashboardLocation)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, keyUsageDashboardLocation)
}

func (s *KeyUsageDashboardService) GetDashboard(
	ctx context.Context,
	userID int64,
	rangeName string,
	filters KeyUsageDashboardFilters,
) (*KeyUsageDashboardResponse, error) {
	window, err := ResolveKeyUsageDashboardWindow(rangeName, s.now())
	if err != nil {
		return nil, err
	}
	filters.Model = strings.TrimSpace(filters.Model)
	cacheKey := keyUsageDashboardCacheKey(userID, window.Range, filters)
	if response, ok := s.getCachedDashboard(cacheKey); ok {
		return response, nil
	}

	if s.repo == nil {
		return nil, errors.New("key usage dashboard repository is not configured")
	}
	if s.apiKeys == nil {
		return nil, errors.New("api key service is not configured")
	}

	response, err := s.buildDashboard(ctx, userID, window, filters)
	if err != nil {
		return nil, fmt.Errorf("get key usage dashboard: %w", err)
	}
	s.setCachedDashboard(cacheKey, response)
	return response, nil
}

func (s *KeyUsageDashboardService) GetKeyDetail(
	ctx context.Context,
	apiKey *APIKey,
	rangeName string,
	filters KeyUsageDashboardFilters,
) (*KeyUsageDashboardDetailResponse, error) {
	if apiKey == nil {
		return nil, errors.New("api key is nil")
	}
	window, err := ResolveKeyUsageDashboardWindow(rangeName, s.now())
	if err != nil {
		return nil, err
	}
	if s.repo == nil {
		return nil, errors.New("key usage dashboard repository is not configured")
	}
	filters.Model = strings.TrimSpace(filters.Model)

	scope := KeyUsageDashboardScope{
		UserID:    apiKey.UserID,
		APIKeyID:  apiKey.ID,
		StartTime: window.Start,
		EndTime:   window.End,
		Filters:   filters,
	}
	previousScope := scope
	previousScope.StartTime = window.PreviousStart
	previousScope.EndTime = window.PreviousEnd

	var (
		hourRows   []KeyUsageDashboardHourRow
		peakRows   []KeyUsageDashboardPeakRow
		errorRows  []KeyUsageDashboardErrorRow
		modelRows  []KeyUsageDashboardModelRow
		effortRows []KeyUsageDashboardEffortRow
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var queryErr error
		hourRows, queryErr = s.repo.GetKeyHourBuckets(groupCtx, scope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		peakRows, queryErr = s.repo.GetPeakRPM(groupCtx, scope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		errorRows, queryErr = s.repo.GetErrorAggregates(groupCtx, scope, previousScope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		modelRows, queryErr = s.repo.GetModelAggregates(groupCtx, scope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		effortRows, queryErr = s.repo.GetEffortAggregates(groupCtx, scope)
		return queryErr
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	totals := sumKeyUsageDashboardRows(hourRows)
	errorRow := keyUsageDashboardErrorByKey(errorRows, apiKey.ID)
	peakRPM := keyUsageDashboardPeakByKey(peakRows, apiKey.ID)
	response := &KeyUsageDashboardDetailResponse{
		APIKeyID:             apiKey.ID,
		Name:                 apiKey.Name,
		GroupName:            keyUsageDashboardGroupName(apiKey),
		Totals:               totals.metric(),
		CacheHitRate:         totals.cacheHitRate(),
		AvgFirstTokenMs:      totals.avgFirstTokenMs(),
		ErrorRate:            keyUsageDashboardErrorRate(totals.Requests, errorRow.ErrorCount),
		PeakRPM:              peakRPM,
		LargeRequestCount:    errorRow.LargeRequestCount,
		LargeRequestAvgBytes: errorRow.LargeRequestAvgBytes,
		HourOfDay:            keyUsageDashboardHourOfDay(hourRows),
		Models:               buildKeyUsageDashboardDetailModels(modelRows),
		Efforts:              buildKeyUsageDashboardEfforts(effortRows),
	}
	return response, nil
}

func (s *KeyUsageDashboardService) buildDashboard(
	ctx context.Context,
	userID int64,
	window KeyUsageDashboardWindow,
	filters KeyUsageDashboardFilters,
) (*KeyUsageDashboardResponse, error) {
	scope := KeyUsageDashboardScope{
		UserID:    userID,
		StartTime: window.Start,
		EndTime:   window.End,
		Filters:   filters,
	}
	previousScope := scope
	previousScope.StartTime = window.PreviousStart
	previousScope.EndTime = window.PreviousEnd
	// Spike alerts always compare today with the preceding seven full days.
	todayStart := startOfKeyUsageDashboardDay(window.End)
	comparisonPreviousStart := todayStart.AddDate(0, 0, -7)

	var (
		hourRows          []KeyUsageDashboardHourRow
		previousTotals    KeyUsageDashboardAggregate
		peakRows          []KeyUsageDashboardPeakRow
		errorRows         []KeyUsageDashboardErrorRow
		modelRows         []KeyUsageDashboardModelRow
		weekRows          []KeyUsageDashboardHourRow
		comparisonRows    []KeyUsageDashboardCostComparisonRow
		apiKeys           []APIKey
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var queryErr error
		hourRows, queryErr = s.repo.GetKeyHourBuckets(groupCtx, scope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		previousTotals, queryErr = s.repo.GetUsageTotals(groupCtx, previousScope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		peakRows, queryErr = s.repo.GetPeakRPM(groupCtx, scope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		errorRows, queryErr = s.repo.GetErrorAggregates(groupCtx, scope, previousScope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		modelRows, queryErr = s.repo.GetModelAggregates(groupCtx, scope)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		comparisonRows, queryErr = s.repo.GetKeyCostComparisons(
			groupCtx,
			scope,
			todayStart,
			window.End,
			comparisonPreviousStart,
		)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		apiKeys, queryErr = s.apiKeys.ListAllByUserID(groupCtx, userID)
		return queryErr
	})
	if window.Range == KeyUsageDashboardRangeToday {
		weekScope := scope
		weekScope.StartTime = window.Start.AddDate(0, 0, -6)
		weekScope.EndTime = window.End
		group.Go(func() error {
			var queryErr error
			weekRows, queryErr = s.repo.GetKeyHourBuckets(groupCtx, weekScope)
			return queryErr
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if window.Range != KeyUsageDashboardRangeToday {
		weekRows = hourRows
	}

	return buildKeyUsageDashboardResponse(
		window,
		apiKeys,
		hourRows,
		weekRows,
		previousTotals,
		peakRows,
		errorRows,
		modelRows,
		comparisonRows,
		s.now(),
		filters,
	), nil
}

func buildKeyUsageDashboardResponse(
	window KeyUsageDashboardWindow,
	apiKeys []APIKey,
	hourRows []KeyUsageDashboardHourRow,
	weekRows []KeyUsageDashboardHourRow,
	previousTotals KeyUsageDashboardAggregate,
	peakRows []KeyUsageDashboardPeakRow,
	errorRows []KeyUsageDashboardErrorRow,
	modelRows []KeyUsageDashboardModelRow,
	comparisonRows []KeyUsageDashboardCostComparisonRow,
	now time.Time,
	filters KeyUsageDashboardFilters,
) *KeyUsageDashboardResponse {
	totals := sumKeyUsageDashboardRows(hourRows)
	keyTotals := keyUsageDashboardTotalsByKey(hourRows)
	hoursByKey := keyUsageDashboardHoursByKey(hourRows)
	seriesByKey := keyUsageDashboardSeriesByKey(hourRows, window)
	errorsByKey := keyUsageDashboardErrorsByKey(errorRows)
	peaksByKey := keyUsageDashboardPeaksByKey(peakRows)

	keys := make([]KeyUsageDashboardKey, 0, len(apiKeys))
	activeKeys := int64(0)
	for index := range apiKeys {
		apiKey := apiKeys[index]
		keyTotal := keyTotals[apiKey.ID]
		errorRow := errorsByKey[apiKey.ID]
		series := keyUsageDashboardSeries(seriesByKey[apiKey.ID], window)
		keys = append(keys, KeyUsageDashboardKey{
			APIKeyID:        apiKey.ID,
			Name:            apiKey.Name,
			GroupName:       keyUsageDashboardGroupName(&apiKey),
			Status:          apiKey.Status,
			LastUsedAt:      apiKey.LastUsedAt,
			Totals:          keyTotal.metric(),
			CostPerRequest:  keyUsageDashboardCostPerRequest(keyTotal),
			CacheHitRate:    keyTotal.cacheHitRate(),
			AvgFirstTokenMs: keyTotal.avgFirstTokenMs(),
			ErrorCount:      errorRow.ErrorCount,
			ErrorRate:       keyUsageDashboardErrorRate(keyTotal.Requests, errorRow.ErrorCount),
			PeakRPM:         peaksByKey[apiKey.ID],
			HourOfDay:       keyUsageDashboardHourBuckets(hoursByKey[apiKey.ID]),
			Series:          series,
		})
		if keyTotal.Requests > 0 {
			activeKeys++
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].Totals.Cost == keys[j].Totals.Cost {
			return keys[i].APIKeyID < keys[j].APIKeyID
		}
		return keys[i].Totals.Cost > keys[j].Totals.Cost
	})

	week := buildKeyUsageDashboardWeek(weekRows)
	peakHours := buildKeyUsageDashboardPeakHours(hourRows)
	modelSummary := buildKeyUsageDashboardModelSummary(modelRows, keys)
	totalErrorCount := sumKeyUsageDashboardErrors(errorRows)
	previousErrorCount := sumKeyUsageDashboardPreviousErrors(errorRows)

	response := &KeyUsageDashboardResponse{
		Range:                 window.Range,
		Start:                 window.Start.Format(time.RFC3339),
		End:                   window.End.Format(time.RFC3339),
		Totals:                totals.metric(),
		PrevTotals:            previousTotals.metric(),
		ActiveKeys:            activeKeys,
		TotalKeys:             int64(len(apiKeys)),
		AvgCostPerRequest:     keyUsageDashboardCostPerRequest(totals),
		PrevAvgCostPerRequest: keyUsageDashboardCostPerRequest(previousTotals),
		CacheHitRate:          totals.cacheHitRate(),
		PrevCacheHitRate:      previousTotals.cacheHitRate(),
		ErrorRate:             keyUsageDashboardErrorRate(totals.Requests, totalErrorCount),
		PrevErrorRate:         keyUsageDashboardErrorRate(previousTotals.Requests, previousErrorCount),
		PeakHours:             peakHours,
		Alerts:                buildKeyUsageDashboardAlerts(apiKeys, keyTotals, errorsByKey, comparisonRows, now, filters),
		Keys:                  keys,
		SeriesLabels:          append([]string(nil), window.SeriesLabels...),
		Week:                  week,
		Models:                modelSummary,
	}
	return response
}

func keyUsageDashboardCacheKey(userID int64, rangeName string, filters KeyUsageDashboardFilters) string {
	return fmt.Sprintf("%d|%s|%d|%s", userID, rangeName, filters.GroupID, filters.Model)
}

func (s *KeyUsageDashboardService) getCachedDashboard(key string) (*KeyUsageDashboardResponse, bool) {
	now := s.now()
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if !now.Before(entry.expiresAt) {
		s.cacheMu.Lock()
		if current, exists := s.cache[key]; exists && !now.Before(current.expiresAt) {
			delete(s.cache, key)
		}
		s.cacheMu.Unlock()
		return nil, false
	}
	return entry.response, true
}

func (s *KeyUsageDashboardService) setCachedDashboard(key string, response *KeyUsageDashboardResponse) {
	if response == nil || s.cacheTTL <= 0 {
		return
	}
	now := s.now()
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.cache == nil {
		s.cache = make(map[string]keyUsageDashboardCacheEntry)
	}
	for existing, entry := range s.cache {
		if !now.Before(entry.expiresAt) {
			delete(s.cache, existing)
		}
	}
	s.cache[key] = keyUsageDashboardCacheEntry{
		response:  response,
		expiresAt: now.Add(s.cacheTTL),
	}
	for s.maxEntries > 0 && len(s.cache) > s.maxEntries {
		oldestKey := ""
		var oldestExpiry time.Time
		for existing, entry := range s.cache {
			if existing == key {
				continue
			}
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = existing
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.cache, oldestKey)
	}
}

func (a KeyUsageDashboardAggregate) tokens() int64 {
	return a.InputTokens + a.OutputTokens + a.CacheCreationTokens + a.CacheReadTokens
}

func (a KeyUsageDashboardAggregate) metric() KeyUsageDashboardMetric {
	return KeyUsageDashboardMetric{
		Cost:     a.Cost,
		Requests: a.Requests,
		Tokens:   a.tokens(),
	}
}

func (a KeyUsageDashboardAggregate) cacheHitRate() float64 {
	denominator := a.InputTokens + a.CacheReadTokens + a.CacheCreationTokens
	if denominator <= 0 {
		return 0
	}
	return float64(a.CacheReadTokens) / float64(denominator)
}

func (a KeyUsageDashboardAggregate) avgFirstTokenMs() int64 {
	if a.FirstTokenSamples <= 0 {
		return 0
	}
	return int64(math.Round(float64(a.FirstTokenMsTotal) / float64(a.FirstTokenSamples)))
}

func (a *KeyUsageDashboardAggregate) add(other KeyUsageDashboardAggregate) {
	a.Requests += other.Requests
	a.InputTokens += other.InputTokens
	a.OutputTokens += other.OutputTokens
	a.CacheCreationTokens += other.CacheCreationTokens
	a.CacheReadTokens += other.CacheReadTokens
	a.Cost += other.Cost
	a.FirstTokenMsTotal += other.FirstTokenMsTotal
	a.FirstTokenSamples += other.FirstTokenSamples
}

func keyUsageDashboardCostPerRequest(aggregate KeyUsageDashboardAggregate) float64 {
	if aggregate.Requests <= 0 {
		return 0
	}
	return aggregate.Cost / float64(aggregate.Requests)
}

func keyUsageDashboardErrorRate(requests, errors int64) float64 {
	total := requests + errors
	if total <= 0 {
		return 0
	}
	return float64(errors) / float64(total)
}

func keyUsageDashboardGroupName(apiKey *APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Name
}

func sumKeyUsageDashboardRows(rows []KeyUsageDashboardHourRow) KeyUsageDashboardAggregate {
	var total KeyUsageDashboardAggregate
	for index := range rows {
		total.add(rows[index].KeyUsageDashboardAggregate)
	}
	return total
}

func keyUsageDashboardTotalsByKey(rows []KeyUsageDashboardHourRow) map[int64]KeyUsageDashboardAggregate {
	totals := make(map[int64]KeyUsageDashboardAggregate)
	for index := range rows {
		total := totals[rows[index].APIKeyID]
		total.add(rows[index].KeyUsageDashboardAggregate)
		totals[rows[index].APIKeyID] = total
	}
	return totals
}

func keyUsageDashboardHoursByKey(rows []KeyUsageDashboardHourRow) map[int64][24]KeyUsageDashboardAggregate {
	hours := make(map[int64][24]KeyUsageDashboardAggregate)
	for index := range rows {
		row := rows[index]
		if row.Hour < 0 || row.Hour > 23 {
			continue
		}
		values := hours[row.APIKeyID]
		values[row.Hour].add(row.KeyUsageDashboardAggregate)
		hours[row.APIKeyID] = values
	}
	return hours
}

func keyUsageDashboardHourBuckets(values [24]KeyUsageDashboardAggregate) []KeyUsageDashboardMetric {
	result := make([]KeyUsageDashboardMetric, 24)
	for hour := 0; hour < 24; hour++ {
		result[hour] = values[hour].metric()
	}
	return result
}

func keyUsageDashboardSeriesByKey(
	rows []KeyUsageDashboardHourRow,
	window KeyUsageDashboardWindow,
) map[int64]map[string]KeyUsageDashboardAggregate {
	series := make(map[int64]map[string]KeyUsageDashboardAggregate)
	for index := range rows {
		row := rows[index]
		if series[row.APIKeyID] == nil {
			series[row.APIKeyID] = make(map[string]KeyUsageDashboardAggregate)
		}
		key := keyUsageDashboardSeriesBucketKey(row, window.Range)
		total := series[row.APIKeyID][key]
		total.add(row.KeyUsageDashboardAggregate)
		series[row.APIKeyID][key] = total
	}
	return series
}

func keyUsageDashboardSeriesBucketKey(row KeyUsageDashboardHourRow, rangeName string) string {
	if rangeName == KeyUsageDashboardRangeToday {
		return fmt.Sprintf("%s|%02d", row.Date, row.Hour)
	}
	return row.Date
}

func keyUsageDashboardSeries(
	values map[string]KeyUsageDashboardAggregate,
	window KeyUsageDashboardWindow,
) []KeyUsageDashboardMetric {
	result := make([]KeyUsageDashboardMetric, len(window.SeriesLabels))
	if window.Range == KeyUsageDashboardRangeToday {
		for hour := 0; hour < 24 && hour < len(result); hour++ {
			key := fmt.Sprintf("%s|%02d", window.Start.Format("2006-01-02"), hour)
			result[hour] = values[key].metric()
		}
		return result
	}

	for index := range window.SeriesLabels {
		date := keyUsageDashboardSeriesLabelDate(window, index)
		if date == "" {
			continue
		}
		result[index] = values[date].metric()
	}
	return result
}

func keyUsageDashboardSeriesLabelDate(window KeyUsageDashboardWindow, index int) string {
	start := startOfKeyUsageDashboardDay(window.Start)
	return start.AddDate(0, 0, index).Format("2006-01-02")
}

func buildKeyUsageDashboardWeek(rows []KeyUsageDashboardHourRow) [][]KeyUsageDashboardMetric {
	var totals [7][24]KeyUsageDashboardAggregate
	for index := range rows {
		row := rows[index]
		if row.Hour < 0 || row.Hour > 23 {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", row.Date, keyUsageDashboardLocation)
		if err != nil {
			continue
		}
		weekday := (int(day.Weekday()) + 6) % 7
		totals[weekday][row.Hour].add(row.KeyUsageDashboardAggregate)
	}

	week := make([][]KeyUsageDashboardMetric, 7)
	for weekday := 0; weekday < 7; weekday++ {
		week[weekday] = make([]KeyUsageDashboardMetric, 24)
		for hour := 0; hour < 24; hour++ {
			week[weekday][hour] = totals[weekday][hour].metric()
		}
	}
	return week
}

func buildKeyUsageDashboardPeakHours(rows []KeyUsageDashboardHourRow) []KeyUsageDashboardPeakHour {
	var totals [24]KeyUsageDashboardAggregate
	for index := range rows {
		row := rows[index]
		if row.Hour >= 0 && row.Hour <= 23 {
			totals[row.Hour].add(row.KeyUsageDashboardAggregate)
		}
	}
	hours := make([]KeyUsageDashboardPeakHour, 0, 24)
	for hour := 0; hour < 24; hour++ {
		if totals[hour].Requests <= 0 {
			continue
		}
		hours = append(hours, KeyUsageDashboardPeakHour{
			Hour:     hour,
			Requests: totals[hour].Requests,
		})
	}
	sort.SliceStable(hours, func(i, j int) bool {
		if hours[i].Requests == hours[j].Requests {
			return hours[i].Hour < hours[j].Hour
		}
		return hours[i].Requests > hours[j].Requests
	})
	if len(hours) > 3 {
		hours = hours[:3]
	}
	return hours
}

func buildKeyUsageDashboardModelSummary(
	rows []KeyUsageDashboardModelRow,
	keys []KeyUsageDashboardKey,
) KeyUsageDashboardModels {
	modelTotals := make(map[string]KeyUsageDashboardAggregate)
	for index := range rows {
		total := modelTotals[rows[index].Model]
		total.add(rows[index].KeyUsageDashboardAggregate)
		modelTotals[rows[index].Model] = total
	}
	names := make([]string, 0, len(modelTotals))
	for model := range modelTotals {
		names = append(names, model)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if modelTotals[names[i]].Cost == modelTotals[names[j]].Cost {
			return names[i] < names[j]
		}
		return modelTotals[names[i]].Cost > modelTotals[names[j]].Cost
	})
	if len(names) > keyUsageDashboardModelLimit {
		names = names[:keyUsageDashboardModelLimit]
	}
	nameIndex := make(map[string]int, len(names))
	for index, name := range names {
		nameIndex[name] = index
	}

	allowedKeys := make(map[int64]struct{}, len(keys))
	keyOrder := make(map[int64]int, len(keys))
	keyCells := make(map[int64][]KeyUsageDashboardAggregate)
	for index := range keys {
		allowedKeys[keys[index].APIKeyID] = struct{}{}
		keyOrder[keys[index].APIKeyID] = index
		if keys[index].Totals.Requests > 0 {
			keyCells[keys[index].APIKeyID] = make([]KeyUsageDashboardAggregate, len(names))
		}
	}
	for index := range rows {
		row := rows[index]
		modelIndex, ok := nameIndex[row.Model]
		if !ok {
			continue
		}
		if _, ok := allowedKeys[row.APIKeyID]; !ok {
			continue
		}
		cells, ok := keyCells[row.APIKeyID]
		if !ok {
			continue
		}
		cells[modelIndex].add(row.KeyUsageDashboardAggregate)
		keyCells[row.APIKeyID] = cells
	}
	modelKeys := make([]KeyUsageDashboardModelKey, 0, len(keyCells))
	for apiKeyID, cells := range keyCells {
		metrics := make([]KeyUsageDashboardMetric, len(names))
		for index := range cells {
			metrics[index] = cells[index].metric()
		}
		modelKeys = append(modelKeys, KeyUsageDashboardModelKey{
			APIKeyID: apiKeyID,
			Cells:    metrics,
		})
	}
	sort.SliceStable(modelKeys, func(i, j int) bool {
		return keyOrder[modelKeys[i].APIKeyID] < keyOrder[modelKeys[j].APIKeyID]
	})
	return KeyUsageDashboardModels{
		Names: names,
		Keys:  modelKeys,
	}
}

func buildKeyUsageDashboardDetailModels(rows []KeyUsageDashboardModelRow) []KeyUsageDashboardModelBreakdown {
	totals := make(map[string]KeyUsageDashboardAggregate)
	for index := range rows {
		total := totals[rows[index].Model]
		total.add(rows[index].KeyUsageDashboardAggregate)
		totals[rows[index].Model] = total
	}
	type modelTotal struct {
		name      string
		aggregate KeyUsageDashboardAggregate
	}
	ordered := make([]modelTotal, 0, len(totals))
	for name, total := range totals {
		ordered = append(ordered, modelTotal{name: name, aggregate: total})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].aggregate.Cost == ordered[j].aggregate.Cost {
			return ordered[i].name < ordered[j].name
		}
		return ordered[i].aggregate.Cost > ordered[j].aggregate.Cost
	})

	result := make([]KeyUsageDashboardModelBreakdown, 0, len(ordered))
	if len(ordered) <= 6 {
		for index := range ordered {
			result = append(result, KeyUsageDashboardModelBreakdown{
				Model:   ordered[index].name,
				Metrics: ordered[index].aggregate.metric(),
			})
		}
		return result
	}

	var other KeyUsageDashboardAggregate
	for index := range ordered {
		if index < 5 {
			result = append(result, KeyUsageDashboardModelBreakdown{
				Model:   ordered[index].name,
				Metrics: ordered[index].aggregate.metric(),
			})
			continue
		}
		other.add(ordered[index].aggregate)
	}
	result = append(result, KeyUsageDashboardModelBreakdown{
		Model:   "其他",
		Metrics: other.metric(),
	})
	return result
}

func buildKeyUsageDashboardEfforts(rows []KeyUsageDashboardEffortRow) []KeyUsageDashboardEffort {
	result := make([]KeyUsageDashboardEffort, 0, len(rows))
	for index := range rows {
		effort := strings.TrimSpace(rows[index].Effort)
		if effort == "" {
			effort = "unknown"
		}
		result = append(result, KeyUsageDashboardEffort{
			Effort:   effort,
			Requests: rows[index].Requests,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Effort == "unknown" {
			return false
		}
		if result[j].Effort == "unknown" {
			return true
		}
		if result[i].Requests == result[j].Requests {
			return result[i].Effort < result[j].Effort
		}
		return result[i].Requests > result[j].Requests
	})
	return result
}

func keyUsageDashboardErrorByKey(rows []KeyUsageDashboardErrorRow, apiKeyID int64) KeyUsageDashboardErrorRow {
	for index := range rows {
		if rows[index].APIKeyID == apiKeyID {
			return rows[index]
		}
	}
	return KeyUsageDashboardErrorRow{}
}

func keyUsageDashboardPeakByKey(rows []KeyUsageDashboardPeakRow, apiKeyID int64) int64 {
	for index := range rows {
		if rows[index].APIKeyID == apiKeyID {
			return rows[index].PeakRPM
		}
	}
	return 0
}

func keyUsageDashboardHourOfDay(rows []KeyUsageDashboardHourRow) []KeyUsageDashboardMetric {
	var hours [24]KeyUsageDashboardAggregate
	for index := range rows {
		row := rows[index]
		if row.Hour >= 0 && row.Hour <= 23 {
			hours[row.Hour].add(row.KeyUsageDashboardAggregate)
		}
	}
	return keyUsageDashboardHourBuckets(hours)
}

func keyUsageDashboardErrorsByKey(rows []KeyUsageDashboardErrorRow) map[int64]KeyUsageDashboardErrorRow {
	result := make(map[int64]KeyUsageDashboardErrorRow, len(rows))
	for index := range rows {
		result[rows[index].APIKeyID] = rows[index]
	}
	return result
}

func keyUsageDashboardPeaksByKey(rows []KeyUsageDashboardPeakRow) map[int64]int64 {
	result := make(map[int64]int64, len(rows))
	for index := range rows {
		result[rows[index].APIKeyID] = rows[index].PeakRPM
	}
	return result
}

func sumKeyUsageDashboardErrors(rows []KeyUsageDashboardErrorRow) int64 {
	var total int64
	for index := range rows {
		total += rows[index].ErrorCount
	}
	return total
}

func sumKeyUsageDashboardPreviousErrors(rows []KeyUsageDashboardErrorRow) int64 {
	var total int64
	for index := range rows {
		total += rows[index].PreviousErrorCount
	}
	return total
}

func buildKeyUsageDashboardAlerts(
	apiKeys []APIKey,
	totals map[int64]KeyUsageDashboardAggregate,
	errorsByKey map[int64]KeyUsageDashboardErrorRow,
	comparisons []KeyUsageDashboardCostComparisonRow,
	now time.Time,
	filters KeyUsageDashboardFilters,
) []KeyUsageDashboardAlert {
	names := make(map[int64]string, len(apiKeys))
	for index := range apiKeys {
		names[apiKeys[index].ID] = apiKeys[index].Name
	}

	highErrorAlerts := make([]KeyUsageDashboardAlert, 0)
	largeRequestAlerts := make([]KeyUsageDashboardAlert, 0)
	spikeAlerts := make([]KeyUsageDashboardAlert, 0)
	idleAlerts := make([]KeyUsageDashboardAlert, 0)

	alertKeyIDs := make(map[int64]struct{}, len(totals)+len(errorsByKey))
	for apiKeyID := range totals {
		if _, exists := names[apiKeyID]; exists {
			alertKeyIDs[apiKeyID] = struct{}{}
		}
	}
	for apiKeyID := range errorsByKey {
		if _, exists := names[apiKeyID]; exists {
			alertKeyIDs[apiKeyID] = struct{}{}
		}
	}
	for apiKeyID := range alertKeyIDs {
		total := totals[apiKeyID]
		errorRow := errorsByKey[apiKeyID]
		rate := keyUsageDashboardErrorRate(total.Requests, errorRow.ErrorCount)
		if errorRow.ErrorCount >= 20 && rate >= 0.05 {
			highErrorAlerts = append(highErrorAlerts, KeyUsageDashboardAlert{
				Type:       "high_error_rate",
				APIKeyID:   apiKeyID,
				APIKeyName: names[apiKeyID],
				Value:      rate,
				Count:      errorRow.ErrorCount,
			})
		}
		if errorRow.LargeRequestCount >= 5 {
			largeRequestAlerts = append(largeRequestAlerts, KeyUsageDashboardAlert{
				Type:       "large_request",
				APIKeyID:   apiKeyID,
				APIKeyName: names[apiKeyID],
				Value:      errorRow.LargeRequestAvgBytes,
				Count:      errorRow.LargeRequestCount,
			})
		}
	}

	for index := range comparisons {
		row := comparisons[index]
		if _, exists := names[row.APIKeyID]; !exists {
			continue
		}
		previousDailyAverage := row.PreviousCost / 7
		if row.TodayCost < 5 {
			continue
		}
		if previousDailyAverage > 0 && row.TodayCost < previousDailyAverage*2 {
			continue
		}
		value := row.TodayCost
		if previousDailyAverage > 0 {
			value = row.TodayCost / previousDailyAverage
		}
		spikeAlerts = append(spikeAlerts, KeyUsageDashboardAlert{
			Type:       "usage_spike",
			APIKeyID:   row.APIKeyID,
			APIKeyName: names[row.APIKeyID],
			Value:      value,
		})
	}

	if filters.GroupID <= 0 && strings.TrimSpace(filters.Model) == "" {
		cutoff := now.AddDate(0, 0, -14)
		idleKeys := make([]APIKey, 0)
		for index := range apiKeys {
			apiKey := apiKeys[index]
			if apiKey.Status != StatusAPIKeyActive {
				continue
			}
			if !apiKey.CreatedAt.Before(cutoff) {
				continue
			}
			if apiKey.LastUsedAt == nil || apiKey.LastUsedAt.Before(cutoff) {
				idleKeys = append(idleKeys, apiKey)
			}
		}
		if len(idleKeys) > 0 {
			sort.SliceStable(idleKeys, func(i, j int) bool {
				return idleKeys[i].ID < idleKeys[j].ID
			})
			idleAlerts = append(idleAlerts, KeyUsageDashboardAlert{
				Type:       "idle",
				APIKeyID:   idleKeys[0].ID,
				APIKeyName: idleKeys[0].Name,
				Count:      int64(len(idleKeys)),
			})
		}
	}

	sort.SliceStable(highErrorAlerts, func(i, j int) bool {
		if highErrorAlerts[i].Value == highErrorAlerts[j].Value {
			return highErrorAlerts[i].Count > highErrorAlerts[j].Count
		}
		return highErrorAlerts[i].Value > highErrorAlerts[j].Value
	})
	sort.SliceStable(largeRequestAlerts, func(i, j int) bool {
		if largeRequestAlerts[i].Value == largeRequestAlerts[j].Value {
			return largeRequestAlerts[i].Count > largeRequestAlerts[j].Count
		}
		return largeRequestAlerts[i].Value > largeRequestAlerts[j].Value
	})
	sort.SliceStable(spikeAlerts, func(i, j int) bool {
		return spikeAlerts[i].Value > spikeAlerts[j].Value
	})

	highErrorAlerts = limitKeyUsageDashboardAlerts(highErrorAlerts)
	largeRequestAlerts = limitKeyUsageDashboardAlerts(largeRequestAlerts)
	spikeAlerts = limitKeyUsageDashboardAlerts(spikeAlerts)
	result := make([]KeyUsageDashboardAlert, 0, len(highErrorAlerts)+len(largeRequestAlerts)+len(spikeAlerts)+len(idleAlerts))
	result = append(result, highErrorAlerts...)
	result = append(result, largeRequestAlerts...)
	result = append(result, spikeAlerts...)
	result = append(result, idleAlerts...)
	return result
}

func limitKeyUsageDashboardAlerts(alerts []KeyUsageDashboardAlert) []KeyUsageDashboardAlert {
	if len(alerts) > keyUsageDashboardAlertLimit {
		return alerts[:keyUsageDashboardAlertLimit]
	}
	return alerts
}
