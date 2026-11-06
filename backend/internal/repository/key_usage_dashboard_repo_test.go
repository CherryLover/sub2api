//go:build unit

package repository

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestKeyUsageDashboardSQLShape(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	start := time.Date(2026, 10, 8, 0, 0, 0, 0, loc)
	end := time.Date(2026, 11, 6, 15, 30, 0, 0, loc)
	current := service.KeyUsageDashboardScope{
		UserID:    42,
		APIKeyID:  7,
		StartTime: start,
		EndTime:   end,
		Filters: service.KeyUsageDashboardFilters{
			GroupID: 12,
			Model:   "gpt-5",
		},
	}
	previous := current
	previous.StartTime = start.AddDate(0, 0, -30)
	previous.EndTime = end.AddDate(0, 0, -30)

	tests := []struct {
		name      string
		fragments []string
	}{
		{
			name: "totals",
			fragments: []string{
				"FROM usage_logs ul",
				"WHERE ul.user_id = $1",
				"ul.api_key_id = $2",
				"ul.group_id = $3",
				"COALESCE(NULLIF(BTRIM(ul.requested_model), ''), ul.model) = $4",
				"ul.created_at >= $5",
				"ul.created_at < $6",
			},
		},
		{
			name: "hour buckets",
			fragments: []string{
				"TO_CHAR(ul.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')",
				"EXTRACT(HOUR FROM ul.created_at AT TIME ZONE 'Asia/Shanghai')",
				"GROUP BY",
				"ul.api_key_id",
			},
		},
		{
			name: "peak rpm",
			fragments: []string{
				"date_trunc('minute'",
				"COUNT(*)::bigint AS request_count",
				"GROUP BY ul.api_key_id, minute_bucket",
				"MAX(per.request_count)::bigint",
			},
		},
		{
			name: "errors",
			fragments: []string{
				"FROM ops_error_logs e",
				"e.status_code >= 400",
				"NOT COALESCE(e.is_count_tokens, false)",
				"COALESCE(NULLIF(BTRIM(e.requested_model), ''), e.model) = $4",
				"e.request_body_bytes >= 5242880",
				"COUNT(*) FILTER",
				"COALESCE(AVG(e.request_body_bytes) FILTER",
				"COALESCE(e.api_key_id, 0)::bigint AS api_key_id",
				"GROUP BY COALESCE(e.api_key_id, 0)",
				"ORDER BY COALESCE(e.api_key_id, 0)",
			},
		},
		{
			name: "models",
			fragments: []string{
				"AS model_name",
				"FROM usage_logs ul",
				"GROUP BY ul.api_key_id, model_name",
			},
		},
		{
			name: "efforts",
			fragments: []string{
				"COALESCE(NULLIF(BTRIM(ul.reasoning_effort), ''), 'unknown')",
				"GROUP BY effort",
				"ORDER BY requests DESC, effort ASC",
			},
		},
		{
			name: "cost comparisons",
			fragments: []string{
				"AS today_cost",
				"AS previous_cost",
				"FILTER",
				"GROUP BY ul.api_key_id",
			},
		},
	}

	assertBalanced := func(t *testing.T, query string) {
		t.Helper()
		require.Equal(t, strings.Count(query, "("), strings.Count(query, ")"), "unbalanced SQL:\n%s", query)
	}

	for index := range tests {
		test := tests[index]
		t.Run(test.name, func(t *testing.T) {
			query, args := buildKeyUsageDashboardQueryForTest(test.name, current, previous, start)
			require.NotEmpty(t, args)
			assertBalanced(t, query)
			for _, fragment := range test.fragments {
				require.Contains(t, query, fragment)
			}
			assertSequentialPlaceholders(t, query, len(args))
		})
	}
}

func TestKeyUsageDashboardSQLArgumentTypes(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	start := time.Date(2026, 10, 8, 0, 0, 0, 0, loc)
	end := time.Date(2026, 11, 6, 15, 30, 0, 0, loc)
	previousStart := start.AddDate(0, 0, -30)
	current := service.KeyUsageDashboardScope{
		UserID:    42,
		APIKeyID:  7,
		StartTime: start,
		EndTime:   end,
		Filters: service.KeyUsageDashboardFilters{
			GroupID: 12,
			Model:   "gpt-5",
		},
	}
	previous := current
	previous.StartTime = previousStart
	previous.EndTime = end.AddDate(0, 0, -30)

	usageArgs := []any{int64(42), int64(7), int64(12), "gpt-5", start, end}
	cases := []struct {
		name  string
		build func() (string, []any)
		want  []any
	}{
		{
			name:  "totals",
			build: func() (string, []any) { return buildKeyUsageDashboardTotalsQuery(current) },
			want:  usageArgs,
		},
		{
			name:  "hour buckets",
			build: func() (string, []any) { return buildKeyUsageDashboardHourBucketsQuery(current) },
			want:  usageArgs,
		},
		{
			name:  "peak rpm",
			build: func() (string, []any) { return buildKeyUsageDashboardPeakQuery(current) },
			want:  usageArgs,
		},
		{
			name:  "errors",
			build: func() (string, []any) { return buildKeyUsageDashboardErrorQuery(current, previous) },
			want: []any{
				int64(42), int64(7), int64(12), "gpt-5",
				start, end, previousStart, previous.EndTime,
			},
		},
		{
			name:  "models",
			build: func() (string, []any) { return buildKeyUsageDashboardModelsQuery(current) },
			want:  usageArgs,
		},
		{
			name:  "efforts",
			build: func() (string, []any) { return buildKeyUsageDashboardEffortsQuery(current) },
			want:  usageArgs,
		},
		{
			name:  "cost comparisons",
			build: func() (string, []any) { return buildKeyUsageDashboardCostComparisonsQuery(current, previousStart) },
			want:  []any{int64(42), int64(7), int64(12), "gpt-5", previousStart, start, end},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, args := test.build()
			require.Equal(t, test.want, args)
		})
	}
}

func buildKeyUsageDashboardQueryForTest(
	name string,
	current service.KeyUsageDashboardScope,
	previous service.KeyUsageDashboardScope,
	previousStart time.Time,
) (string, []any) {
	switch name {
	case "totals":
		return buildKeyUsageDashboardTotalsQuery(current)
	case "hour buckets":
		return buildKeyUsageDashboardHourBucketsQuery(current)
	case "peak rpm":
		return buildKeyUsageDashboardPeakQuery(current)
	case "errors":
		return buildKeyUsageDashboardErrorQuery(current, previous)
	case "models":
		return buildKeyUsageDashboardModelsQuery(current)
	case "efforts":
		return buildKeyUsageDashboardEffortsQuery(current)
	case "cost comparisons":
		return buildKeyUsageDashboardCostComparisonsQuery(current, previousStart)
	default:
		panic("unknown query")
	}
}

func testPlaceholderNumbers(t *testing.T, query string) []int {
	t.Helper()
	matches := regexp.MustCompile(`\$([0-9]+)`).FindAllStringSubmatch(query, -1)
	numbers := make([]int, 0, len(matches))
	for _, match := range matches {
		number, err := strconv.Atoi(match[1])
		require.NoError(t, err)
		numbers = append(numbers, number)
	}
	return numbers
}

func assertSequentialPlaceholders(t *testing.T, query string, argCount int) {
	t.Helper()
	seen := make(map[int]struct{})
	for _, number := range testPlaceholderNumbers(t, query) {
		require.GreaterOrEqual(t, number, 1)
		require.LessOrEqual(t, number, argCount)
		seen[number] = struct{}{}
	}
	for number := 1; number <= argCount; number++ {
		_, ok := seen[number]
		require.True(t, ok, "placeholder $%d is not referenced", number)
	}
}
