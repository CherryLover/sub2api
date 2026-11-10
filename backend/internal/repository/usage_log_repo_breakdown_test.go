//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestResolveEndpointColumn(t *testing.T) {
	tests := []struct {
		endpointType string
		want         string
	}{
		{"inbound", "ul.inbound_endpoint"},
		{"upstream", "ul.upstream_endpoint"},
		{"path", "ul.inbound_endpoint || ' -> ' || ul.upstream_endpoint"},
		{"", "ul.inbound_endpoint"},        // default
		{"unknown", "ul.inbound_endpoint"}, // fallback
	}

	for _, tc := range tests {
		t.Run(tc.endpointType, func(t *testing.T) {
			got := resolveEndpointColumn(tc.endpointType)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestResolveModelDimensionExpression(t *testing.T) {
	tests := []struct {
		modelType string
		want      string
	}{
		{usagestats.ModelSourceRequested, "COALESCE(NULLIF(TRIM(requested_model), ''), model)"},
		{usagestats.ModelSourceUpstream, "COALESCE(NULLIF(TRIM(upstream_model), ''), model)"},
		{usagestats.ModelSourceMapping, "(COALESCE(NULLIF(TRIM(requested_model), ''), model) || ' -> ' || COALESCE(NULLIF(TRIM(upstream_model), ''), model))"},
		{"", "COALESCE(NULLIF(TRIM(requested_model), ''), model)"},
		{"invalid", "COALESCE(NULLIF(TRIM(requested_model), ''), model)"},
	}

	for _, tc := range tests {
		t.Run(tc.modelType, func(t *testing.T) {
			got := resolveModelDimensionExpression(tc.modelType)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetUserBreakdownStatsRequestTypeIncludesLegacyFallback(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)

	legacyFilter := `(ul.request_type = $3 OR (ul.request_type = 0 AND ul.stream = TRUE AND ul.openai_ws_mode = FALSE))`
	mock.ExpectQuery(regexp.QuoteMeta(legacyFilter)).
		WithArgs(start, end, requestType).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "email", "requests", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "cost", "actual_cost", "account_cost",
		}))

	rows, err := repo.GetUserBreakdownStats(context.Background(), start, end, usagestats.UserBreakdownDimension{
		RequestType: &requestType,
	}, 0)

	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAPIKeyBreakdownStatsAggregatesByKey(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta("GROUP BY ul.api_key_id, k.name, ul.user_id, u.email")).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"api_key_id", "name", "user_id", "email", "requests", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "cost", "actual_cost",
		}).AddRow(7, "king jin", 2, "it@xthings.com", 3124, 39057003, 4293749, 1087045375, 1130396127, 796.10, 796.10))

	rows, err := repo.GetAPIKeyBreakdownStats(context.Background(), start, end, usagestats.UserBreakdownDimension{}, 20)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(7), rows[0].APIKeyID)
	require.Equal(t, "king jin", rows[0].Name)
	require.Equal(t, "it@xthings.com", rows[0].Email)
	require.Equal(t, int64(3124), rows[0].Requests)
	require.InDelta(t, 796.10, rows[0].ActualCost, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAPIKeyBreakdownStatsAppliesFiltersAndLimit(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)
	stream := true

	legacyFilter := `(ul.request_type = $4 OR (ul.request_type = 0 AND ul.stream = TRUE AND ul.openai_ws_mode = FALSE))`
	mock.ExpectQuery(regexp.QuoteMeta(legacyFilter)).
		WithArgs(start, end, int64(42), requestType, stream).
		WillReturnRows(sqlmock.NewRows([]string{
			"api_key_id", "name", "user_id", "email", "requests", "input_tokens", "output_tokens",
			"cache_tokens", "total_tokens", "cost", "actual_cost",
		}))

	rows, err := repo.GetAPIKeyBreakdownStats(context.Background(), start, end, usagestats.UserBreakdownDimension{
		UserID:      42,
		RequestType: &requestType,
		Stream:      &stream,
		SortBy:      "requests",
	}, 5)

	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBreakdownModelSharesGroupsByModel(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta("ul.user_id = ANY($3)")).
		WithArgs(start, end, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"dimension_id", "model", "requests", "tokens", "cost", "actual_cost",
		}).
			AddRow(2, "gpt-6-astra", 2236, 446150000, 581.92, 581.92).
			AddRow(2, "gpt-5.6-sol", 1621, 189580000, 174.75, 174.75))

	shares, err := repo.GetBreakdownModelShares(context.Background(), start, end,
		usagestats.UserBreakdownDimension{}, usagestats.BreakdownModelByUser, []int64{2})

	require.NoError(t, err)
	require.Len(t, shares[2], 2)
	require.Equal(t, "gpt-6-astra", shares[2][0].Model)
	require.Equal(t, int64(2236), shares[2][0].Requests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBreakdownModelSharesRejectsUnknownDimension(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	shares, err := repo.GetBreakdownModelShares(context.Background(), time.Now(), time.Now(),
		usagestats.UserBreakdownDimension{}, usagestats.BreakdownModelDimension("DROP TABLE usage_logs"), []int64{1})

	require.NoError(t, err)
	require.Empty(t, shares)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBreakdownModelSharesSkipsEmptyIDs(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	shares, err := repo.GetBreakdownModelShares(context.Background(), time.Now(), time.Now(),
		usagestats.UserBreakdownDimension{}, usagestats.BreakdownModelByAPIKey, nil)

	require.NoError(t, err)
	require.Empty(t, shares)
	require.NoError(t, mock.ExpectationsWereMet())
}
