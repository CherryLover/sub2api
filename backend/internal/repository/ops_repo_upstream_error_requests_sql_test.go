//go:build unit

package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func requireBalancedSQLParens(t *testing.T, sql string) {
	t.Helper()
	depth := 0
	for _, r := range sql {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			require.GreaterOrEqual(t, depth, 0, sql)
		}
	}
	require.Zero(t, depth, sql)
}

func TestUpstreamErrorRequestsWhere_InclusionSemantics(t *testing.T) {
	start := time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	where, _ := upstreamErrorRequestsWhere(&service.OpsUpstreamErrorRequestFilter{StartTime: start, EndTime: end})

	require.Contains(t, where, "EXISTS (SELECT 1 FROM jsonb_array_elements")
	require.Contains(t, where, "ev->>'upstream_status_code'")
	require.Contains(t, where, ">= 400")
	require.Contains(t, where, "COALESCE(ev->>'kind','') <> ''")
	require.Contains(t, where, "COALESCE(ev->>'message','') <> ''")
	require.Contains(t, where, "COALESCE(ev->>'detail','') <> ''")
	require.Contains(t, where, "jsonb_array_length(")
	require.Contains(t, where, "LOWER(TRIM(COALESCE(e.error_phase,''))) IN ('upstream','network','account_auth')")
	require.Contains(t, where, "COALESCE(e.upstream_status_code,e.status_code,0) >= 400")
	require.NotContains(t, where, `('502','503')`)
	require.NotContains(t, strings.ToLower(where), "error_phase = 'internal'")
	requireBalancedSQLParens(t, where)
}

func TestBuildUpstreamErrorRequestSummarySQL_Structure(t *testing.T) {
	start := time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsUpstreamErrorRequestFilter{
		StartTime: start,
		EndTime:   end,
		APIKeyID:  7,
		AccountID: 9,
		StatusCode: 503,
		FinalStatusCode: 502,
		Outcome: "recovered",
	}

	summarySQL, summaryArgs, failedSQL, failedArgs := buildUpstreamErrorRequestSummarySQL(filter)

	require.Equal(t, []any{start, end, int64(7), "9", 503, 502}, summaryArgs)
	require.Equal(t, summaryArgs, failedArgs)
	require.Contains(t, summarySQL, "WITH matched AS MATERIALIZED")
	require.Contains(t, summarySQL, "SELECT e.status_code,e.upstream_errors")
	require.Contains(t, summarySQL, "FROM ops_error_logs e WHERE")
	require.Contains(t, summarySQL, "COUNT(*) FILTER")
	require.Contains(t, summarySQL, "COALESCE(SUM(")
	require.Contains(t, summarySQL, "jsonb_array_elements(")
	require.Contains(t, summarySQL, "FROM matched e")
	require.Contains(t, summarySQL, "ev->>'account_id' = $4")
	require.Contains(t, summarySQL, "e.account_id::text = $4")
	require.NotContains(t, summarySQL, "ev->>'account_id' = $4::text")
	require.NotContains(t, summarySQL, "e.account_id = $4")
	require.Contains(t, failedSQL, "FROM ops_error_logs e WHERE")
	require.Contains(t, failedSQL, "ev->>'account_id' = $4")
	require.Contains(t, failedSQL, "e.account_id::text = $4")
	require.NotContains(t, failedSQL, "ev->>'account_id' = $4::text")
	require.NotContains(t, failedSQL, "e.account_id = $4")
	require.Contains(t, failedSQL, "NOT (COALESCE(e.status_code,0) >= 200")
	require.Contains(t, failedSQL, "GROUP BY e.status_code")
	require.Contains(t, failedSQL, "ORDER BY COUNT(*) DESC,e.status_code ASC")
	require.Contains(t, failedSQL, "LIMIT 10")
	requireBalancedSQLParens(t, summarySQL)
	requireBalancedSQLParens(t, failedSQL)
}

func TestBuildUpstreamErrorRequestTrendSQL_Structure(t *testing.T) {
	start := time.Date(2026, 10, 27, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	end := start.AddDate(0, 0, 7)
	filter := &service.OpsUpstreamErrorRequestFilter{
		StartTime: start,
		EndTime:   end,
		StatusCode: 429,
		Outcome:    "failed",
	}

	query, args := buildUpstreamErrorRequestTrendSQL(filter)

	require.Equal(t, []any{start.UTC(), end.UTC(), 429}, args)
	require.Contains(t, query, "TO_CHAR((e.created_at AT TIME ZONE 'Asia/Shanghai')::date, 'YYYY-MM-DD')")
	require.Contains(t, query, "COUNT(*)")
	require.Contains(t, query, "COUNT(*) FILTER")
	require.Contains(t, query, "FROM ops_error_logs e WHERE")
	require.Contains(t, query, "GROUP BY 1")
	require.Contains(t, query, "ORDER BY 1 ASC")
	requireBalancedSQLParens(t, query)
}
