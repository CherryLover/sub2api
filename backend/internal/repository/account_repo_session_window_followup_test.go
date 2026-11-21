package repository

import (
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
	"time"
)

func TestActiveSessionWindowUpdateIncludesBothBounds(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	end := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	// Ent emits updated_at followed by the two session fields in schema order.
	mock.ExpectExec(`UPDATE "accounts" SET .*"session_window_start" = .*"session_window_end" = .*WHERE "accounts"."id" =`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WithArgs(service.SchedulerOutboxEventAccountChanged, int64(731), nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	repo := newAccountRepositoryWithSQL(client, db, nil)
	require.NoError(t, repo.UpdateSessionWindowEnd(t.Context(), 731, end))
	require.NoError(t, mock.ExpectationsWereMet())
}
