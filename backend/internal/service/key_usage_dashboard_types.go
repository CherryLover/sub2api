package service

import (
	"context"
	"time"
)

const (
	KeyUsageDashboardRangeToday = "today"
	KeyUsageDashboardRange7d    = "7d"
	KeyUsageDashboardRange30d   = "30d"

	KeyUsageDashboardCacheTTL          = 60 * time.Second
	KeyUsageDashboardCacheMaxEntries   = 256
	KeyUsageDashboardLargeRequestBytes int64 = 5 * 1024 * 1024
)

// KeyUsageDashboardFilters are the optional user-facing filters.
type KeyUsageDashboardFilters struct {
	GroupID int64
	Model   string
}

// KeyUsageDashboardScope is the shared SQL scope for one dashboard query.
type KeyUsageDashboardScope struct {
	UserID    int64
	APIKeyID  int64
	StartTime time.Time
	EndTime   time.Time
	Filters   KeyUsageDashboardFilters
}

// KeyUsageDashboardWindow contains the selected and previous periods.
type KeyUsageDashboardWindow struct {
	Range         string
	Start         time.Time
	End           time.Time
	PreviousStart time.Time
	PreviousEnd   time.Time
	Days          int
	SeriesLabels  []string
}

type KeyUsageDashboardAggregate struct {
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	Cost                float64
	FirstTokenMsTotal   int64
	FirstTokenSamples   int64
}

type KeyUsageDashboardHourRow struct {
	APIKeyID             int64
	Date                 string
	Hour                 int
	KeyUsageDashboardAggregate
}

type KeyUsageDashboardPeakRow struct {
	APIKeyID int64
	PeakRPM  int64
}

type KeyUsageDashboardErrorRow struct {
	APIKeyID             int64
	ErrorCount           int64
	PreviousErrorCount   int64
	LargeRequestCount    int64
	LargeRequestAvgBytes float64
}

type KeyUsageDashboardModelRow struct {
	APIKeyID             int64
	Model                string
	KeyUsageDashboardAggregate
}

type KeyUsageDashboardEffortRow struct {
	Effort   string
	Requests int64
}

type KeyUsageDashboardCostComparisonRow struct {
	APIKeyID     int64
	TodayCost    float64
	PreviousCost float64
}

// KeyUsageDashboardRepository provides the aggregated dashboard data.
type KeyUsageDashboardRepository interface {
	GetUsageTotals(ctx context.Context, scope KeyUsageDashboardScope) (KeyUsageDashboardAggregate, error)
	GetKeyHourBuckets(ctx context.Context, scope KeyUsageDashboardScope) ([]KeyUsageDashboardHourRow, error)
	GetPeakRPM(ctx context.Context, scope KeyUsageDashboardScope) ([]KeyUsageDashboardPeakRow, error)
	GetErrorAggregates(ctx context.Context, current, previous KeyUsageDashboardScope) ([]KeyUsageDashboardErrorRow, error)
	GetModelAggregates(ctx context.Context, scope KeyUsageDashboardScope) ([]KeyUsageDashboardModelRow, error)
	GetEffortAggregates(ctx context.Context, scope KeyUsageDashboardScope) ([]KeyUsageDashboardEffortRow, error)
	GetKeyCostComparisons(ctx context.Context, scope KeyUsageDashboardScope, todayStart, todayEnd, previousStart time.Time) ([]KeyUsageDashboardCostComparisonRow, error)
}

type KeyUsageDashboardMetric struct {
	Cost     float64 `json:"cost"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
}

type KeyUsageDashboardAlert struct {
	Type       string  `json:"type"`
	APIKeyID   int64   `json:"api_key_id"`
	APIKeyName string  `json:"api_key_name"`
	Value      float64 `json:"value"`
	Count      int64   `json:"count"`
}

type KeyUsageDashboardPeakHour struct {
	Hour     int   `json:"hour"`
	Requests int64 `json:"requests"`
}

type KeyUsageDashboardKey struct {
	APIKeyID        int64                     `json:"api_key_id"`
	Name            string                    `json:"name"`
	GroupName       string                    `json:"group_name"`
	Status          string                    `json:"status"`
	LastUsedAt      *time.Time                `json:"last_used_at"`
	Totals          KeyUsageDashboardMetric   `json:"totals"`
	CostPerRequest  float64                   `json:"cost_per_request"`
	CacheHitRate    float64                   `json:"cache_hit_rate"`
	AvgFirstTokenMs int64                     `json:"avg_first_token_ms"`
	ErrorCount      int64                     `json:"error_count"`
	ErrorRate       float64                   `json:"error_rate"`
	PeakRPM         int64                     `json:"peak_rpm"`
	HourOfDay       []KeyUsageDashboardMetric `json:"hour_of_day"`
	Series          []KeyUsageDashboardMetric `json:"series"`
}

type KeyUsageDashboardModelKey struct {
	APIKeyID int64                     `json:"api_key_id"`
	Cells    []KeyUsageDashboardMetric `json:"cells"`
}

type KeyUsageDashboardModels struct {
	Names []string                    `json:"names"`
	Keys  []KeyUsageDashboardModelKey `json:"keys"`
}

type KeyUsageDashboardResponse struct {
	Range                 string                      `json:"range"`
	Start                 string                      `json:"start"`
	End                   string                      `json:"end"`
	Totals                KeyUsageDashboardMetric     `json:"totals"`
	PrevTotals            KeyUsageDashboardMetric     `json:"prev_totals"`
	ActiveKeys            int64                       `json:"active_keys"`
	TotalKeys             int64                       `json:"total_keys"`
	AvgCostPerRequest     float64                     `json:"avg_cost_per_request"`
	PrevAvgCostPerRequest float64                     `json:"prev_avg_cost_per_request"`
	CacheHitRate          float64                     `json:"cache_hit_rate"`
	PrevCacheHitRate      float64                     `json:"prev_cache_hit_rate"`
	ErrorRate             float64                     `json:"error_rate"`
	PrevErrorRate         float64                     `json:"prev_error_rate"`
	PeakHours             []KeyUsageDashboardPeakHour `json:"peak_hours"`
	Alerts                []KeyUsageDashboardAlert    `json:"alerts"`
	Keys                  []KeyUsageDashboardKey      `json:"keys"`
	SeriesLabels          []string                    `json:"series_labels"`
	Week                  [][]KeyUsageDashboardMetric `json:"week"`
	Models                KeyUsageDashboardModels     `json:"models"`
}

type KeyUsageDashboardModelBreakdown struct {
	Model   string                  `json:"model"`
	Metrics KeyUsageDashboardMetric `json:"metrics"`
}

type KeyUsageDashboardEffort struct {
	Effort   string `json:"effort"`
	Requests int64  `json:"requests"`
}

type KeyUsageDashboardDetailResponse struct {
	APIKeyID             int64                             `json:"api_key_id"`
	Name                 string                            `json:"name"`
	GroupName            string                            `json:"group_name"`
	Totals               KeyUsageDashboardMetric           `json:"totals"`
	CacheHitRate         float64                           `json:"cache_hit_rate"`
	AvgFirstTokenMs      int64                             `json:"avg_first_token_ms"`
	ErrorRate            float64                           `json:"error_rate"`
	PeakRPM              int64                             `json:"peak_rpm"`
	LargeRequestCount    int64                             `json:"large_request_count"`
	LargeRequestAvgBytes float64                           `json:"large_request_avg_bytes"`
	HourOfDay            []KeyUsageDashboardMetric         `json:"hour_of_day"`
	Models               []KeyUsageDashboardModelBreakdown `json:"models"`
	Efforts              []KeyUsageDashboardEffort         `json:"efforts"`
}
