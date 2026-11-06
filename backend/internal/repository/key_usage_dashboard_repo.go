package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const keyUsageDashboardLocationSQL = "Asia/Shanghai"

type keyUsageDashboardRepository struct {
	db *sql.DB
}

// NewKeyUsageDashboardRepository creates the aggregated dashboard repository.
func NewKeyUsageDashboardRepository(db *sql.DB) service.KeyUsageDashboardRepository {
	return &keyUsageDashboardRepository{db: db}
}

func keyUsageDashboardModelExpression(alias string) string {
	return fmt.Sprintf(
		"COALESCE(NULLIF(BTRIM(%s.requested_model), ''), %s.model)",
		alias,
		alias,
	)
}

func keyUsageDashboardErrorModelExpression(alias string) string {
	return fmt.Sprintf(
		"COALESCE(NULLIF(BTRIM(%s.requested_model), ''), %s.model)",
		alias,
		alias,
	)
}

func appendKeyUsageDashboardUsageFilters(
	conditions []string,
	args []any,
	scope service.KeyUsageDashboardScope,
	alias string,
) ([]string, []any) {
	if scope.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("%s.user_id = $%d", alias, len(args)+1))
		args = append(args, scope.UserID)
	}
	if scope.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("%s.api_key_id = $%d", alias, len(args)+1))
		args = append(args, scope.APIKeyID)
	}
	if scope.Filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("%s.group_id = $%d", alias, len(args)+1))
		args = append(args, scope.Filters.GroupID)
	}
	if model := strings.TrimSpace(scope.Filters.Model); model != "" {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", keyUsageDashboardModelExpression(alias), len(args)+1))
		args = append(args, model)
	}
	return conditions, args
}

func appendKeyUsageDashboardErrorFilters(
	conditions []string,
	args []any,
	scope service.KeyUsageDashboardScope,
) ([]string, []any) {
	if scope.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("e.user_id = $%d", len(args)+1))
		args = append(args, scope.UserID)
	}
	if scope.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("e.api_key_id = $%d", len(args)+1))
		args = append(args, scope.APIKeyID)
	}
	if scope.Filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("e.group_id = $%d", len(args)+1))
		args = append(args, scope.Filters.GroupID)
	}
	if model := strings.TrimSpace(scope.Filters.Model); model != "" {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", keyUsageDashboardErrorModelExpression("e"), len(args)+1))
		args = append(args, model)
	}
	return conditions, args
}

func keyUsageDashboardAggregateColumns(alias string) string {
	return fmt.Sprintf(`COUNT(*)::bigint AS requests,
			COALESCE(SUM(%s.input_tokens), 0)::bigint AS input_tokens,
			COALESCE(SUM(%s.output_tokens), 0)::bigint AS output_tokens,
			COALESCE(SUM(%s.cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
			COALESCE(SUM(%s.cache_read_tokens), 0)::bigint AS cache_read_tokens,
			COALESCE(SUM(%s.actual_cost), 0)::double precision AS cost,
			COALESCE(SUM(%s.first_token_ms) FILTER (WHERE %s.first_token_ms IS NOT NULL), 0)::bigint AS first_token_ms_total,
			COUNT(*) FILTER (WHERE %s.first_token_ms IS NOT NULL)::bigint AS first_token_samples`,
		alias, alias, alias, alias, alias, alias, alias, alias,
	)
}

func buildKeyUsageDashboardTotalsQuery(scope service.KeyUsageDashboardScope) (string, []any) {
	conditions, args := appendKeyUsageDashboardUsageFilters(nil, nil, scope, "ul")
	conditions = append(conditions, fmt.Sprintf("ul.created_at >= $%d", len(args)+1))
	args = append(args, scope.StartTime)
	conditions = append(conditions, fmt.Sprintf("ul.created_at < $%d", len(args)+1))
	args = append(args, scope.EndTime)

	query := fmt.Sprintf(`SELECT
			%s
		FROM usage_logs ul
		WHERE %s`, keyUsageDashboardAggregateColumns("ul"), strings.Join(conditions, " AND "))
	return query, args
}

func buildKeyUsageDashboardHourBucketsQuery(scope service.KeyUsageDashboardScope) (string, []any) {
	conditions, args := appendKeyUsageDashboardUsageFilters(nil, nil, scope, "ul")
	conditions = append(conditions, fmt.Sprintf("ul.created_at >= $%d", len(args)+1))
	args = append(args, scope.StartTime)
	conditions = append(conditions, fmt.Sprintf("ul.created_at < $%d", len(args)+1))
	args = append(args, scope.EndTime)

	query := fmt.Sprintf(`SELECT
			ul.api_key_id,
			TO_CHAR(ul.created_at AT TIME ZONE '%s', 'YYYY-MM-DD') AS bucket_date,
			EXTRACT(HOUR FROM ul.created_at AT TIME ZONE '%s')::integer AS bucket_hour,
			%s
		FROM usage_logs ul
		WHERE %s
		GROUP BY
			ul.api_key_id,
			TO_CHAR(ul.created_at AT TIME ZONE '%s', 'YYYY-MM-DD'),
			EXTRACT(HOUR FROM ul.created_at AT TIME ZONE '%s')
		ORDER BY ul.api_key_id, bucket_date, bucket_hour`,
		keyUsageDashboardLocationSQL,
		keyUsageDashboardLocationSQL,
		keyUsageDashboardAggregateColumns("ul"),
		strings.Join(conditions, " AND "),
		keyUsageDashboardLocationSQL,
		keyUsageDashboardLocationSQL,
	)
	return query, args
}

func buildKeyUsageDashboardPeakQuery(scope service.KeyUsageDashboardScope) (string, []any) {
	conditions, args := appendKeyUsageDashboardUsageFilters(nil, nil, scope, "ul")
	conditions = append(conditions, fmt.Sprintf("ul.created_at >= $%d", len(args)+1))
	args = append(args, scope.StartTime)
	conditions = append(conditions, fmt.Sprintf("ul.created_at < $%d", len(args)+1))
	args = append(args, scope.EndTime)

	query := fmt.Sprintf(`SELECT per.api_key_id, MAX(per.request_count)::bigint AS peak_rpm
		FROM (
			SELECT
				ul.api_key_id,
				date_trunc('minute', ul.created_at AT TIME ZONE '%s') AS minute_bucket,
				COUNT(*)::bigint AS request_count
			FROM usage_logs ul
			WHERE %s
			GROUP BY ul.api_key_id, minute_bucket
		) per
		GROUP BY per.api_key_id
		ORDER BY per.api_key_id`,
		keyUsageDashboardLocationSQL,
		strings.Join(conditions, " AND "),
	)
	return query, args
}

func buildKeyUsageDashboardErrorQuery(
	current service.KeyUsageDashboardScope,
	previous service.KeyUsageDashboardScope,
) (string, []any) {
	conditions := []string{
		"e.status_code >= 400",
		"NOT COALESCE(e.is_count_tokens, false)",
	}
	var args []any
	conditions, args = appendKeyUsageDashboardErrorFilters(conditions, args, current)

	currentStartPosition := len(args) + 1
	args = append(args, current.StartTime)
	currentEndPosition := len(args) + 1
	args = append(args, current.EndTime)
	previousStartPosition := len(args) + 1
	args = append(args, previous.StartTime)
	previousEndPosition := len(args) + 1
	args = append(args, previous.EndTime)

	conditions = append(
		conditions,
		fmt.Sprintf(
			"e.created_at >= LEAST($%d::timestamptz, $%d::timestamptz)",
			previousStartPosition,
			currentStartPosition,
		),
		fmt.Sprintf(
			"e.created_at < GREATEST($%d::timestamptz, $%d::timestamptz)",
			previousEndPosition,
			currentEndPosition,
		),
	)

	query := fmt.Sprintf(`SELECT
			COALESCE(e.api_key_id, 0)::bigint AS api_key_id,
			COALESCE(COUNT(*) FILTER (
				WHERE e.created_at >= $%d::timestamptz
				  AND e.created_at < $%d::timestamptz
			), 0)::bigint AS error_count,
			COALESCE(COUNT(*) FILTER (
				WHERE e.created_at >= $%d::timestamptz
				  AND e.created_at < $%d::timestamptz
			), 0)::bigint AS previous_error_count,
			COALESCE(COUNT(*) FILTER (
				WHERE e.created_at >= $%d::timestamptz
				  AND e.created_at < $%d::timestamptz
				  AND e.request_body_bytes >= %d
			), 0)::bigint AS large_request_count,
			COALESCE(AVG(e.request_body_bytes) FILTER (
				WHERE e.created_at >= $%d::timestamptz
				  AND e.created_at < $%d::timestamptz
				  AND e.request_body_bytes >= %d
			), 0)::double precision AS large_request_avg_bytes
		FROM ops_error_logs e
		WHERE %s
		GROUP BY COALESCE(e.api_key_id, 0)
		ORDER BY COALESCE(e.api_key_id, 0)`,
		currentStartPosition,
		currentEndPosition,
		previousStartPosition,
		previousEndPosition,
		currentStartPosition,
		currentEndPosition,
		service.KeyUsageDashboardLargeRequestBytes,
		currentStartPosition,
		currentEndPosition,
		service.KeyUsageDashboardLargeRequestBytes,
		strings.Join(conditions, " AND "),
	)
	return query, args
}

func buildKeyUsageDashboardModelsQuery(scope service.KeyUsageDashboardScope) (string, []any) {
	conditions, args := appendKeyUsageDashboardUsageFilters(nil, nil, scope, "ul")
	conditions = append(conditions, fmt.Sprintf("ul.created_at >= $%d", len(args)+1))
	args = append(args, scope.StartTime)
	conditions = append(conditions, fmt.Sprintf("ul.created_at < $%d", len(args)+1))
	args = append(args, scope.EndTime)
	modelExpression := keyUsageDashboardModelExpression("ul")

	query := fmt.Sprintf(`SELECT
			ul.api_key_id,
			%s AS model_name,
			%s
		FROM usage_logs ul
		WHERE %s
		GROUP BY ul.api_key_id, model_name
		ORDER BY ul.api_key_id, model_name`,
		modelExpression,
		keyUsageDashboardAggregateColumns("ul"),
		strings.Join(conditions, " AND "),
	)
	return query, args
}

func buildKeyUsageDashboardEffortsQuery(scope service.KeyUsageDashboardScope) (string, []any) {
	conditions, args := appendKeyUsageDashboardUsageFilters(nil, nil, scope, "ul")
	conditions = append(conditions, fmt.Sprintf("ul.created_at >= $%d", len(args)+1))
	args = append(args, scope.StartTime)
	conditions = append(conditions, fmt.Sprintf("ul.created_at < $%d", len(args)+1))
	args = append(args, scope.EndTime)

	query := fmt.Sprintf(`SELECT
			COALESCE(NULLIF(BTRIM(ul.reasoning_effort), ''), 'unknown') AS effort,
			COUNT(*)::bigint AS requests
		FROM usage_logs ul
		WHERE %s
		GROUP BY effort
		ORDER BY requests DESC, effort ASC`,
		strings.Join(conditions, " AND "),
	)
	return query, args
}

func buildKeyUsageDashboardCostComparisonsQuery(
	scope service.KeyUsageDashboardScope,
	previousStart time.Time,
) (string, []any) {
	conditions, args := appendKeyUsageDashboardUsageFilters(nil, nil, scope, "ul")
	previousStartPosition := len(args) + 1
	args = append(args, previousStart)
	todayStartPosition := len(args) + 1
	args = append(args, scope.StartTime)
	todayEndPosition := len(args) + 1
	args = append(args, scope.EndTime)

	conditions = append(
		conditions,
		fmt.Sprintf("ul.created_at >= $%d", previousStartPosition),
		fmt.Sprintf("ul.created_at < $%d", todayEndPosition),
	)
	query := fmt.Sprintf(`SELECT
			ul.api_key_id,
			COALESCE(SUM(ul.actual_cost) FILTER (
				WHERE ul.created_at >= $%d
				  AND ul.created_at < $%d
			), 0)::double precision AS today_cost,
			COALESCE(SUM(ul.actual_cost) FILTER (
				WHERE ul.created_at >= $%d
				  AND ul.created_at < $%d
			), 0)::double precision AS previous_cost
		FROM usage_logs ul
		WHERE %s
		GROUP BY ul.api_key_id
		ORDER BY ul.api_key_id`,
		todayStartPosition,
		todayEndPosition,
		previousStartPosition,
		todayStartPosition,
		strings.Join(conditions, " AND "),
	)
	return query, args
}

func (r *keyUsageDashboardRepository) GetUsageTotals(
	ctx context.Context,
	scope service.KeyUsageDashboardScope,
) (service.KeyUsageDashboardAggregate, error) {
	query, args := buildKeyUsageDashboardTotalsQuery(scope)
	var result service.KeyUsageDashboardAggregate
	err := scanSingleRow(
		ctx,
		r.db,
		query,
		args,
		&result.Requests,
		&result.InputTokens,
		&result.OutputTokens,
		&result.CacheCreationTokens,
		&result.CacheReadTokens,
		&result.Cost,
		&result.FirstTokenMsTotal,
		&result.FirstTokenSamples,
	)
	if err != nil {
		return service.KeyUsageDashboardAggregate{}, err
	}
	return result, nil
}

func (r *keyUsageDashboardRepository) GetKeyHourBuckets(
	ctx context.Context,
	scope service.KeyUsageDashboardScope,
) (rows []service.KeyUsageDashboardHourRow, err error) {
	query, args := buildKeyUsageDashboardHourBucketsQuery(scope)
	sqlRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows = nil
		}
	}()
	for sqlRows.Next() {
		var row service.KeyUsageDashboardHourRow
		if scanErr := sqlRows.Scan(
			&row.APIKeyID,
			&row.Date,
			&row.Hour,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.Cost,
			&row.FirstTokenMsTotal,
			&row.FirstTokenSamples,
		); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *keyUsageDashboardRepository) GetPeakRPM(
	ctx context.Context,
	scope service.KeyUsageDashboardScope,
) (rows []service.KeyUsageDashboardPeakRow, err error) {
	query, args := buildKeyUsageDashboardPeakQuery(scope)
	sqlRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows = nil
		}
	}()
	for sqlRows.Next() {
		var row service.KeyUsageDashboardPeakRow
		if scanErr := sqlRows.Scan(&row.APIKeyID, &row.PeakRPM); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *keyUsageDashboardRepository) GetErrorAggregates(
	ctx context.Context,
	current service.KeyUsageDashboardScope,
	previous service.KeyUsageDashboardScope,
) (rows []service.KeyUsageDashboardErrorRow, err error) {
	query, args := buildKeyUsageDashboardErrorQuery(current, previous)
	sqlRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows = nil
		}
	}()
	for sqlRows.Next() {
		var row service.KeyUsageDashboardErrorRow
		if scanErr := sqlRows.Scan(
			&row.APIKeyID,
			&row.ErrorCount,
			&row.PreviousErrorCount,
			&row.LargeRequestCount,
			&row.LargeRequestAvgBytes,
		); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *keyUsageDashboardRepository) GetModelAggregates(
	ctx context.Context,
	scope service.KeyUsageDashboardScope,
) (rows []service.KeyUsageDashboardModelRow, err error) {
	query, args := buildKeyUsageDashboardModelsQuery(scope)
	sqlRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows = nil
		}
	}()
	for sqlRows.Next() {
		var row service.KeyUsageDashboardModelRow
		if scanErr := sqlRows.Scan(
			&row.APIKeyID,
			&row.Model,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.Cost,
			&row.FirstTokenMsTotal,
			&row.FirstTokenSamples,
		); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *keyUsageDashboardRepository) GetEffortAggregates(
	ctx context.Context,
	scope service.KeyUsageDashboardScope,
) (rows []service.KeyUsageDashboardEffortRow, err error) {
	query, args := buildKeyUsageDashboardEffortsQuery(scope)
	sqlRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows = nil
		}
	}()
	for sqlRows.Next() {
		var row service.KeyUsageDashboardEffortRow
		if scanErr := sqlRows.Scan(&row.Effort, &row.Requests); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *keyUsageDashboardRepository) GetKeyCostComparisons(
	ctx context.Context,
	scope service.KeyUsageDashboardScope,
	todayStart, todayEnd, previousStart time.Time,
) (rows []service.KeyUsageDashboardCostComparisonRow, err error) {
	scope.StartTime = todayStart
	scope.EndTime = todayEnd
	query, args := buildKeyUsageDashboardCostComparisonsQuery(scope, previousStart)
	sqlRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows = nil
		}
	}()
	for sqlRows.Next() {
		var row service.KeyUsageDashboardCostComparisonRow
		if scanErr := sqlRows.Scan(&row.APIKeyID, &row.TodayCost, &row.PreviousCost); scanErr != nil {
			return nil, scanErr
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}
