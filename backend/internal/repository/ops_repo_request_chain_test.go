//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var opsRequestChainErrorColumns = []string{
	"id", "created_at", "status_code", "model", "requested_model", "stream", "time_to_first_token_ms",
	"attempt_index", "at_unix_ms", "attempt_account_id", "attempt_account_name", "attempt_platform",
	"attempt_upstream_status_code", "attempt_message", "attempt_kind",
}

var opsRequestChainUsageColumns = []string{"account_id", "account_name", "total_tokens", "total_cost", "first_token_ms"}

// upstream_errors 可能是 NULL、JSON null、甚至非数组；jsonb_array_elements 碰到这些会直接
// 报错，所以必须先 jsonb_typeof 判型。用 LEFT JOIN LATERAL 而不是 CROSS JOIN，
// 是为了让「没有上游尝试」的错误记录也留在结果里，否则整条链路会凭空消失。
func TestOpsRequestChainErrorsSQLShape(t *testing.T) {
	require.Contains(t, opsRequestChainErrorsSQL, "LEFT JOIN LATERAL jsonb_array_elements(")
	require.Contains(t, opsRequestChainErrorsSQL, "CASE WHEN jsonb_typeof(e.upstream_errors) = 'array' THEN e.upstream_errors ELSE '[]'::jsonb END")
	require.Contains(t, opsRequestChainErrorsSQL, "WITH ORDINALITY AS ev(item, idx) ON TRUE")
	require.Contains(t, opsRequestChainErrorsSQL, "WHERE e.client_request_id = $1")
	require.Contains(t, opsRequestChainErrorsSQL, "ORDER BY e.created_at ASC, e.id ASC, ev.idx ASC")
	// 脏数据不该让整个接口 500：数字字段一律先判型再转。
	require.Contains(t, opsRequestChainErrorsSQL, "jsonb_typeof(ev.item->'at_unix_ms') = 'number'")
	require.Contains(t, opsRequestChainErrorsSQL, "jsonb_typeof(ev.item->'account_id') = 'number'")
	require.Contains(t, opsRequestChainErrorsSQL, "jsonb_typeof(ev.item->'upstream_status_code') = 'number'")
}

func TestGetRequestChainSource_MergesAttemptsAndPrefixesUsageRequestID(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	createdAt1 := time.Date(2026, 9, 11, 6, 59, 39, 0, time.UTC)
	createdAt2 := createdAt1.Add(19 * time.Second)
	atMs := createdAt1.UnixMilli()

	mock.ExpectQuery(opsSQLShape("FROM ops_error_logs e", "LEFT JOIN LATERAL jsonb_array_elements(", "WHERE e.client_request_id = $1")).
		WithArgs("req-abc").
		WillReturnRows(sqlmock.NewRows(opsRequestChainErrorColumns).
			AddRow(int64(1), createdAt1, int64(502), "gpt-5.6-sol", "gpt-5.6-sol", true, nil,
				int64(1), atMs, int64(7), "anviz-7", "openai", int64(502), "overloaded", "failover").
			AddRow(int64(1), createdAt1, int64(502), "gpt-5.6-sol", "gpt-5.6-sol", true, nil,
				int64(2), atMs+2000, int64(7), "anviz-7", "openai", int64(502), "overloaded", "failover").
			// 第二条记录没有任何上游尝试：LEFT JOIN 把它保留下来，attempt_index 为 NULL。
			AddRow(int64(2), createdAt2, int64(200), "gpt-5.6-sol", "gpt-5.6-sol", true, int64(18501),
				nil, nil, nil, "", "", nil, "", ""))

	mock.ExpectQuery(opsSQLShape("FROM usage_logs ul", "WHERE ul.request_id = $1")).
		WithArgs("client:req-abc", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(opsRequestChainUsageColumns).
			AddRow(int64(7), "anviz-7", int64(1883), 0.0212, int64(1200)))

	source, err := repo.GetRequestChainSource(context.Background(), "  req-abc  ")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NotNil(t, source)

	require.Len(t, source.Errors, 2, "同一个 id 的两条 ops_error_logs 各成一条记录")

	first := source.Errors[0]
	require.EqualValues(t, 1, first.ID)
	require.True(t, createdAt1.Equal(first.CreatedAt))
	require.Equal(t, 502, first.StatusCode)
	require.Equal(t, "gpt-5.6-sol", first.Model)
	require.True(t, first.Stream)
	require.Nil(t, first.TimeToFirstTokenMs)
	require.Len(t, first.Attempts, 2, "同一条记录展开出的两次尝试挂回同一条记录")
	require.Equal(t, atMs, first.Attempts[0].AtUnixMs)
	require.EqualValues(t, 7, first.Attempts[0].AccountID)
	require.Equal(t, "anviz-7", first.Attempts[0].AccountName)
	require.Equal(t, "openai", first.Attempts[0].Platform)
	require.Equal(t, 502, first.Attempts[0].UpstreamStatusCode)
	require.Equal(t, "failover", first.Attempts[0].Kind)

	second := source.Errors[1]
	require.EqualValues(t, 2, second.ID)
	require.Equal(t, 200, second.StatusCode)
	require.NotNil(t, second.TimeToFirstTokenMs)
	require.EqualValues(t, 18501, *second.TimeToFirstTokenMs)
	require.NotNil(t, second.Attempts, "upstream_errors 为空时 attempts 是空数组而不是 nil")
	require.Empty(t, second.Attempts)

	require.NotNil(t, source.Usage)
	require.EqualValues(t, 7, source.Usage.AccountID)
	require.EqualValues(t, 1883, source.Usage.TotalTokens)
	require.InDelta(t, 0.0212, source.Usage.Cost, 1e-9)
	require.NotNil(t, source.Usage.FirstTokenMs)
	require.EqualValues(t, 1200, *source.Usage.FirstTokenMs)
}

// usage_logs 关联不上就是关联不上，不编造最终结果。
func TestGetRequestChainSource_NoUsageLogKeepsUsageNil(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	createdAt := time.Date(2026, 9, 11, 6, 59, 39, 0, time.UTC)

	mock.ExpectQuery(opsSQLShape("FROM ops_error_logs e")).
		WithArgs("req-failed").
		WillReturnRows(sqlmock.NewRows(opsRequestChainErrorColumns).
			AddRow(int64(9), createdAt, int64(502), "", "", false, nil,
				nil, nil, nil, "", "", nil, "", ""))

	mock.ExpectQuery(opsSQLShape("FROM usage_logs ul")).
		WithArgs("client:req-failed", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(opsRequestChainUsageColumns))

	source, err := repo.GetRequestChainSource(context.Background(), "req-failed")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, source.Errors, 1)
	require.Nil(t, source.Usage)
}

// 没有任何错误记录时不必再去翻 usage_logs。
func TestGetRequestChainSource_NoErrorRowsSkipsUsageQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	mock.ExpectQuery(opsSQLShape("FROM ops_error_logs e")).
		WithArgs("req-unknown").
		WillReturnRows(sqlmock.NewRows(opsRequestChainErrorColumns))

	source, err := repo.GetRequestChainSource(context.Background(), "req-unknown")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Empty(t, source.Errors)
	require.Nil(t, source.Usage)
}

func TestGetRequestChainSource_RejectsEmptyID(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	source, err := repo.GetRequestChainSource(context.Background(), "   ")
	require.Error(t, err)
	require.Nil(t, source)
	require.NoError(t, mock.ExpectationsWereMet())
}

// usage_logs 的查询窗口由错误记录的时间跨度加固定余量得到：分区表上靠它剪枝，
// 余量放到一天是为了绝不会把真实的计费行排除在外。
func TestOpsRequestChainUsageWindow(t *testing.T) {
	start := time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	records := []*service.OpsRequestChainErrorRecord{
		{ID: 2, CreatedAt: end},
		nil,
		{ID: 3, CreatedAt: time.Time{}},
		{ID: 1, CreatedAt: start},
	}
	windowStart, windowEnd := opsRequestChainUsageWindow(records)
	require.True(t, windowStart.Equal(start.Add(-opsRequestChainUsageLookupSlack)))
	require.True(t, windowEnd.Equal(end.Add(opsRequestChainUsageLookupSlack)))

	// 全是零值时间时退回"现在"，不能算出零值窗口把所有行都排除掉。
	fallbackStart, fallbackEnd := opsRequestChainUsageWindow(nil)
	require.False(t, fallbackStart.IsZero())
	require.True(t, fallbackEnd.After(fallbackStart))
}
