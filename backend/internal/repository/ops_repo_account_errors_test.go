package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// opsSQLShape builds a regexp that requires the given SQL fragments in order.
// sqlmock collapses whitespace on both sides before matching.
func opsSQLShape(fragments ...string) string {
	quoted := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		quoted = append(quoted, regexp.QuoteMeta(fragment))
	}
	return strings.Join(quoted, ".*")
}

var opsAccountRecentErrorWhereFragments = []string{
	"FROM ops_error_logs",
	"WHERE account_id = ANY($1)",
	"AND created_at >= $2",
	"AND NOT COALESCE(is_count_tokens, false)",
	"AND (COALESCE(status_code, 0) >= 400 OR COALESCE(upstream_status_code, 0) >= 400)",
}

func TestOpsRepositoryGetAccountRecentErrorSummaries_QueryShapeAndMapping(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	since := time.Date(2026, 9, 10, 8, 45, 0, 0, time.UTC)
	ids := []int64{5, 6, 7}

	countFragments := append([]string{
		"SELECT account_id,",
		"COALESCE(NULLIF(upstream_status_code, 0), status_code, 0) AS status_code,",
		"COUNT(*) AS cnt",
	}, opsAccountRecentErrorWhereFragments...)
	countFragments = append(countFragments, "GROUP BY 1, 2", "ORDER BY 1, 3 DESC, 2 ASC")
	mock.ExpectQuery(opsSQLShape(countFragments...)).
		WithArgs(pq.Array(ids), since).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "status_code", "cnt"}).
			AddRow(int64(6), int64(502), int64(14)).
			AddRow(int64(6), int64(429), int64(4)).
			AddRow(int64(7), int64(500), int64(1)))

	lastFragments := append([]string{
		"SELECT DISTINCT ON (account_id)",
		"COALESCE(status_code, 0) AS status_code,",
		"upstream_status_code,",
		"COALESCE(NULLIF(upstream_error_message, ''), error_message, '') AS message,",
		"COALESCE(NULLIF(requested_model, ''), model, '') AS model",
	}, opsAccountRecentErrorWhereFragments...)
	lastFragments = append(lastFragments, "ORDER BY account_id, created_at DESC, id DESC")
	lastAt6 := time.Date(2026, 9, 10, 8, 59, 30, 0, time.UTC)
	lastAt7 := time.Date(2026, 9, 10, 8, 50, 0, 0, time.UTC)
	longMessage := strings.Repeat("é", 400)
	mock.ExpectQuery(opsSQLShape(lastFragments...)).
		WithArgs(pq.Array(ids), since).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "created_at", "status_code", "upstream_status_code", "message", "model"}).
			AddRow(int64(6), lastAt6, int64(200), int64(502), "Our servers are currently overloaded. Please try again later.", "gpt-5.6-sol").
			AddRow(int64(7), lastAt7, int64(500), nil, longMessage, " gpt-x "))

	got, err := repo.GetAccountRecentErrorSummaries(context.Background(), ids, since)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, got, 2, "accounts without errors are absent")
	require.NotContains(t, got, int64(5))

	s6 := got[6]
	require.NotNil(t, s6)
	require.EqualValues(t, 18, s6.Total)
	require.Len(t, s6.ByStatus, 2)
	require.Equal(t, 502, s6.ByStatus[0].StatusCode)
	require.EqualValues(t, 14, s6.ByStatus[0].Count)
	require.Equal(t, 429, s6.ByStatus[1].StatusCode)
	require.EqualValues(t, 4, s6.ByStatus[1].Count)
	require.NotNil(t, s6.Last)
	require.True(t, lastAt6.Equal(s6.Last.At))
	require.Equal(t, 200, s6.Last.StatusCode)
	require.NotNil(t, s6.Last.UpstreamStatusCode)
	require.Equal(t, 502, *s6.Last.UpstreamStatusCode)
	require.Equal(t, "Our servers are currently overloaded. Please try again later.", s6.Last.Message)
	require.Equal(t, "gpt-5.6-sol", s6.Last.Model)

	s7 := got[7]
	require.NotNil(t, s7)
	require.EqualValues(t, 1, s7.Total)
	require.NotNil(t, s7.Last)
	require.Nil(t, s7.Last.UpstreamStatusCode)
	require.Equal(t, 300, utf8.RuneCountInString(s7.Last.Message), "message is truncated to 300 runes")
	require.Equal(t, "gpt-x", s7.Last.Model)
}

func TestOpsRepositoryGetAccountRecentErrorSummaries_NoErrorsSkipsLastQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	since := time.Date(2026, 9, 10, 8, 45, 0, 0, time.UTC)

	mock.ExpectQuery(opsSQLShape("COUNT(*) AS cnt", "FROM ops_error_logs")).
		WithArgs(pq.Array([]int64{9}), since).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "status_code", "cnt"}))

	got, err := repo.GetAccountRecentErrorSummaries(context.Background(), []int64{9}, since)
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsRepositoryGetAccountRecentErrorSummaries_EmptyIDsDoesNotQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	got, err := repo.GetAccountRecentErrorSummaries(context.Background(), nil, time.Now())
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
