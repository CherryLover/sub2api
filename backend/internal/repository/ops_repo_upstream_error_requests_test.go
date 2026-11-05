//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func upstreamErrorRequestTestRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "client_id", "request_id", "key_id", "key_name", "account_id", "account_name", "model", "reasoning_effort", "request_body_bytes", "status", "events", "upstream_status", "message", "detail", "final_id", "final_name"})
}
func TestUpstreamErrorRequestsRepository_Errors(t *testing.T) {
	for _, stage := range []string{"count", "list", "scan", "rows", "json"} {
		t.Run(stage, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			sentinel := errors.New("database failed")
			count := mock.ExpectQuery("SELECT count")
			if stage == "count" {
				count.WillReturnError(sentinel)
			} else {
				count.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				list := mock.ExpectQuery("WITH page AS")
				switch stage {
				case "list":
					list.WillReturnError(sentinel)
				case "scan":
					list.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				case "rows":
					list.WillReturnRows(upstreamErrorRequestTestRows().AddRow(1, time.Now(), "", "", nil, "", nil, "", "", "", nil, 502, "[]", 502, "", "", nil, "").RowError(0, sentinel))
				case "json":
					list.WillReturnRows(upstreamErrorRequestTestRows().AddRow(1, time.Now(), "", "", nil, "", nil, "", "", "", nil, 502, "not-json", 502, "", "", nil, ""))
				}
			}
			out, err := (&opsRepository{db: db}).ListUpstreamErrorRequests(context.Background(), &service.OpsUpstreamErrorRequestFilter{Page: 1, PageSize: 20})
			require.Nil(t, out)
			require.Error(t, err)
			if stage == "count" || stage == "list" || stage == "rows" {
				require.ErrorIs(t, err, sentinel)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
func TestUpstreamErrorRequestsRepository_PaginationAndHistoricalEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	start := time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mock.ExpectQuery("SELECT count").WithArgs(start, end).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	rows := upstreamErrorRequestTestRows().
		AddRow(1, start, "cid", "rid", 7, "key", 9, "top-level", "model", "high", 4096, 503, `[{"upstream_status_code":"broken"},{"account_id":8,"account_name":"attempt-owner","upstream_status_code":502}]`, 503, "", "", nil, "").
		AddRow(2, start, "", "", nil, "", nil, "", "model", "", nil, 503, "[]", nil, "legacy message", "legacy detail", nil, "")
	mock.ExpectQuery("WITH page AS").WithArgs(start, end, 10, 20).WillReturnRows(rows)
	out, err := (&opsRepository{db: db}).ListUpstreamErrorRequests(context.Background(), &service.OpsUpstreamErrorRequestFilter{Page: 3, PageSize: 10, StartTime: start, EndTime: end})
	require.NoError(t, err)
	require.EqualValues(t, 2, out.Total)
	require.Equal(t, 3, out.Page)
	require.Equal(t, 10, out.PageSize)
	require.Len(t, out.Items, 2)
	item := out.Items[0]
	require.Len(t, item.Attempts, 1)
	require.EqualValues(t, 8, *item.AccountID)
	require.Equal(t, "attempt-owner", item.AccountName)
	require.Equal(t, "high", item.ReasoningEffort)
	require.NotNil(t, item.RequestBodyBytes)
	require.EqualValues(t, 4096, *item.RequestBodyBytes)
	require.Equal(t, "failed", item.Outcome)
	require.Nil(t, item.Final)
	legacy := out.Items[1]
	require.Nil(t, legacy.APIKeyID)
	require.Nil(t, legacy.AccountID)
	require.Nil(t, legacy.Final)
	require.Len(t, legacy.Attempts, 1)
	require.Equal(t, "legacy", legacy.Attempts[0].Kind)
	require.Equal(t, 503, legacy.Attempts[0].UpstreamStatusCode)
	require.Equal(t, start.UnixMilli(), legacy.Attempts[0].AtUnixMs)
	require.Equal(t, "legacy message", legacy.Attempts[0].Message)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestUpstreamErrorRequestsWhere(t *testing.T) {
	start := time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	tests := []struct {
		name       string
		filter     service.OpsUpstreamErrorRequestFilter
		contains   []string
		notContain []string
		args       []any
	}{
		{
			name:   "events plus legacy upstream rows",
			filter: service.OpsUpstreamErrorRequestFilter{StartTime: start, EndTime: end},
			contains: []string{
				"jsonb_array_length(CASE WHEN jsonb_typeof(e.upstream_errors) = 'array'",
				"ev->>'upstream_status_code'",
				">= 400",
				"LOWER(TRIM(COALESCE(e.error_phase,''))) IN ('upstream','network','account_auth')",
				"COALESCE(e.upstream_status_code,e.status_code,0) >= 400",
			},
			notContain: []string{"('502','503')"},
			args:       []any{start, end},
		},
		{
			name: "all filters",
			filter: service.OpsUpstreamErrorRequestFilter{
				StartTime: start, EndTime: end, APIKeyID: 7, AccountID: 9,
				StatusCode: 418, FinalStatusCode: 503, Outcome: "failed",
			},
			contains: []string{
				"e.api_key_id = $3",
				"ev->>'account_id' = $4",
				"e.account_id::text = $4",
				opsUpstreamEventStatusSQL("ev") + " = $5",
				"COALESCE(e.upstream_status_code,e.status_code,0) = $5",
				"e.status_code = $6",
				"NOT (COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300)",
			},
			notContain: []string{
				"ev->>'account_id' = $4::text",
				"e.account_id = $4",
			},
			args: []any{start, end, int64(7), "9", 418, 503},
		},
		{
			name:     "recovered only",
			filter:   service.OpsUpstreamErrorRequestFilter{StartTime: start, EndTime: end, Outcome: "recovered"},
			contains: []string{"COALESCE(e.status_code,0) >= 200 AND COALESCE(e.status_code,0) < 300"},
			args:     []any{start, end},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args := upstreamErrorRequestsWhere(&tt.filter)
			require.Equal(t, tt.args, args)
			for _, part := range tt.contains {
				require.Contains(t, where, part)
			}
			for _, part := range tt.notContain {
				require.NotContains(t, where, part)
			}
		})
	}
}

func TestUpstreamErrorRequestsRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	f := &service.OpsUpstreamErrorRequestFilter{Page: 1, PageSize: 20, StartTime: now.Add(-time.Hour), EndTime: now}
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM ops_error_logs").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// Require the scoped successful usage join in the generated paginated query.
	mock.ExpectQuery("(?s)WITH page AS .*LIMIT.*e.status_code >= 200 AND e.status_code < 300.*ul.api_key_id=e.api_key_id.*ORDER BY e.created_at DESC,e.id DESC").WillReturnRows(upstreamErrorRequestTestRows().AddRow(1, now, "cid", "rid", 7, "key", 8, "failed-account", "model", "medium", 1234, 200, "[]", 502, "legacy error", "legacy detail", 9, "success-account"))
	repo := &opsRepository{db: db}
	out, err := repo.ListUpstreamErrorRequests(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("%#v", out)
	}
	item := out.Items[0]
	if item.Final == nil || item.Final.AccountID != 9 || item.Outcome != "recovered" || len(item.Attempts) != 1 || item.Attempts[0].Detail != "legacy detail" || item.Attempts[0].AccountID != 8 {
		t.Fatalf("%#v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
