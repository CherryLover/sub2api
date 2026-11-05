//go:build unit

package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsErrorKindSummaryAndListUseSharedExpression(t *testing.T) {
	kind := "routing|503|model_rate_limited"
	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{ErrorKind: kind})

	require.Contains(t, where, opsErrorKindExpr)
	require.Len(t, args, 1)
	require.Equal(t, kind, args[0])

	summaryWhere, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{})
	require.Contains(t, buildOpsErrorSummarySQL(summaryWhere), opsErrorKindExpr)
}

func TestSummarizeErrorLogsIgnoresSelectedKindAndMapsRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{})
	expectedSQL := buildOpsErrorSummarySQL(where)
	mock.ExpectQuery(expectedSQL).
		WithArgs().
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "phase", "status_code", "reason", "sample_message",
			"total_count", "failed_count", "recovered_count",
		}).
			AddRow("upstream|529|our servers are currently overloaded. please try again later.", "upstream", 529, "", "Our servers are currently overloaded. Please try again later.", int64(1696), int64(0), int64(1696)).
			AddRow("routing|503|model_rate_limited", "routing", 503, "model_rate_limited", "Service temporarily unavailable", int64(7), int64(7), int64(0)))

	repo := &opsRepository{db: db}
	got, err := repo.SummarizeErrorLogs(context.Background(), &service.OpsErrorLogFilter{
		ErrorKind: "routing|503|model_rate_limited",
	})
	require.NoError(t, err)
	require.Len(t, args, 0)
	require.EqualValues(t, 1703, got.Total)
	require.Len(t, got.Items, 2)
	require.Equal(t, "upstream", got.Items[0].Phase)
	require.Equal(t, 529, got.Items[0].StatusCode)
	require.EqualValues(t, 1696, got.Items[0].Recovered)
	require.Equal(t, "model_rate_limited", got.Items[1].Reason)
	require.EqualValues(t, 7, got.Items[1].Failed)
	require.False(t, strings.Contains(where, "error_kind"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Regression guard: sqlmock compares the generated SQL with itself, so it cannot
// catch malformed SQL. Check the structural mistakes that broke it once.
func TestOpsErrorSummarySQLIsStructurallySound(t *testing.T) {
	where, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{})
	sql := buildOpsErrorSummarySQL(where)
	for name, expr := range map[string]string{
		"normalized message": opsErrorNormalizedMessageExpr,
		"kind":               opsErrorKindExpr,
		"summary":            sql,
	} {
		require.Equal(t, strings.Count(expr, "("), strings.Count(expr, ")"), "unbalanced parentheses in %s expression", name)
	}
	normalizedMaskCount := strings.Count(opsErrorNormalizedMessageExpr, "REGEXP_REPLACE(") -
		strings.Count(opsErrorVisibleMessageExpr, "REGEXP_REPLACE(")
	require.Equal(t, 6, normalizedMaskCount, "one REGEXP_REPLACE per masking pattern")
	// Without HAVING the catch-all aggregate returns an all-NULL row when there is
	// no overflow, which fails to scan into int64.
	require.Contains(t, sql, "HAVING COUNT(*) > 0")
}

func TestBuildOpsErrorLogsWhereMinRequestBodyBytes(t *testing.T) {
	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{MinRequestBodyBytes: 2 * 1024 * 1024})
	require.Contains(t, where, "e.request_body_bytes >= $1")
	require.Equal(t, []any{int64(2 * 1024 * 1024)}, args)

	whereOff, argsOff := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{})
	require.NotContains(t, whereOff, "request_body_bytes")
	require.Empty(t, argsOff)
}
