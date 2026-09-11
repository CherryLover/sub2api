//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// 两条语句必须用逐字相同的状态码分桶表达式，否则拆分会挂到错误的档位上，
// 前端会看到「502 共 14 条，其中已恢复 0」这种自相矛盾的数字。
func TestOpsAccountRecentErrorStatusBucketExprMatchesCountsSQL(t *testing.T) {
	require.Contains(t, opsAccountRecentErrorCountsSQL, opsAccountRecentErrorStatusBucketExpr)
	require.Contains(t, opsAccountRecentErrorOutcomeSQL, opsAccountRecentErrorStatusBucketExpr)
	// 两条语句也必须扫同一批行。
	require.Contains(t, opsAccountRecentErrorCountsSQL, opsAccountRecentErrorWhere)
	require.Contains(t, opsAccountRecentErrorOutcomeSQL, opsAccountRecentErrorWhere)
}

func TestOpsRepositoryGetAccountRecentErrorOutcomes_QueryShapeAndMapping(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	since := time.Date(2026, 9, 11, 6, 45, 0, 0, time.UTC)
	ids := []int64{6, 7}

	fragments := append([]string{
		"SELECT account_id,",
		"COALESCE(NULLIF(upstream_status_code, 0), status_code, 0) AS status_code,",
		"COUNT(*) FILTER (WHERE COALESCE(status_code, 0) BETWEEN 200 AND 299) AS recovered,",
		"COUNT(*) FILTER (WHERE NOT (COALESCE(status_code, 0) BETWEEN 200 AND 299)) AS failed,",
		"MAX(created_at) AS last_created_at,",
		"(array_agg(COALESCE(client_request_id, '') ORDER BY created_at DESC, id DESC))[1] AS last_client_request_id",
	}, opsAccountRecentErrorWhereFragments...)
	fragments = append(fragments, "GROUP BY 1, 2")

	last502 := time.Date(2026, 9, 11, 6, 59, 30, 0, time.UTC)
	last429 := time.Date(2026, 9, 11, 6, 50, 0, 0, time.UTC)
	mock.ExpectQuery(opsSQLShape(fragments...)).
		WithArgs(pq.Array(ids), since).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "status_code", "recovered", "failed", "last_created_at", "last_client_request_id"}).
			AddRow(int64(6), int64(502), int64(13), int64(1), last502, "req-502").
			AddRow(int64(6), int64(429), int64(2), int64(2), last429, "  req-429  ").
			AddRow(int64(7), int64(500), int64(0), int64(1), nil, nil))

	got, err := repo.GetAccountRecentErrorOutcomes(context.Background(), ids, since)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, got, 2)

	account6 := got[6]
	require.NotNil(t, account6)
	require.EqualValues(t, 15, account6.Recovered, "账号级计数是各档之和")
	require.EqualValues(t, 3, account6.Failed)
	require.Len(t, account6.ByStatus, 2)
	require.Equal(t, 502, account6.ByStatus[0].StatusCode)
	require.EqualValues(t, 13, account6.ByStatus[0].Recovered)
	require.EqualValues(t, 1, account6.ByStatus[0].Failed)
	require.True(t, last502.Equal(account6.ByStatus[0].LastCreatedAt))
	require.Equal(t, "req-502", account6.ByStatus[0].LastClientRequestID)
	require.Equal(t, "req-429", account6.ByStatus[1].LastClientRequestID, "两端空白被裁掉")

	account7 := got[7]
	require.NotNil(t, account7)
	require.EqualValues(t, 0, account7.Recovered)
	require.EqualValues(t, 1, account7.Failed)
	require.Len(t, account7.ByStatus, 1)
	require.True(t, account7.ByStatus[0].LastCreatedAt.IsZero(), "NULL 时间不炸")
	require.Empty(t, account7.ByStatus[0].LastClientRequestID, "NULL client_request_id 回空串")
}

func TestOpsRepositoryGetAccountRecentErrorOutcomes_EmptyIDsDoesNotQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	got, err := repo.GetAccountRecentErrorOutcomes(context.Background(), nil, time.Now())
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetAccountRecentErrorOutcomes_NilRepository(t *testing.T) {
	var repo *opsRepository
	got, err := repo.GetAccountRecentErrorOutcomes(context.Background(), []int64{1}, time.Now())
	require.Error(t, err)
	require.Nil(t, got)
}
