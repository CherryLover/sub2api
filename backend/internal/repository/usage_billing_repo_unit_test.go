//go:build unit

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var accountQuotaStateColumns = []string{"quota_used", "quota_limit", "quota_daily_used", "quota_daily_limit", "quota_weekly_used", "quota_weekly_limit", "quota_monthly_used", "quota_monthly_limit"}

func TestAccountQuotaIncrementSQLs_GuardOmittedDimensions(t *testing.T) {
	for _, daily := range []bool{false, true} {
		for _, weekly := range []bool{false, true} {
			for _, monthly := range []bool{false, true} {
				query := accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(daily, weekly, monthly)]
				set, where, ok := strings.Cut(query, "WHERE ")
				require.True(t, ok)
				require.Equal(t, !daily || !weekly || !monthly, strings.Contains(where, accountQuotaExtraIsObjectCond), "daily=%v weekly=%v", daily, weekly)
				require.Equal(t, daily, strings.Contains(set, "'quota_daily_used'"), "daily=%v weekly=%v", daily, weekly)
				require.Equal(t, weekly, strings.Contains(set, "'quota_weekly_used'"), "daily=%v weekly=%v", daily, weekly)
				require.Equal(t, !daily, strings.Contains(where, accountQuotaDailyLimitUnsetCond), "daily=%v weekly=%v", daily, weekly)
				require.Equal(t, !weekly, strings.Contains(where, accountQuotaWeeklyLimitUnsetCond), "daily=%v weekly=%v", daily, weekly)
				require.Equal(t, monthly, strings.Contains(set, "'quota_monthly_used'"))
				require.Equal(t, !monthly, strings.Contains(where, accountQuotaMonthlyLimitUnsetCond))
			}
		}
	}
}

func TestAccountQuotaIncrementSQLs_ReturnEffectiveMonthlyLimit(t *testing.T) {
	require.Contains(t, accountQuotaIncrementReturning, accountQuotaMonthlyEffectiveLimitExpr)
	require.Contains(t, accountQuotaIncrementReturning, "COALESCE(" + accountQuotaMonthlyEffectiveLimitExpr + ", 0)")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, "(extra->>'quota_monthly_prorated_until')::timestamptz > NOW()")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, "quota_monthly_prorated_limit")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, "quota_monthly_limit")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, "COALESCE(extra->>'quota_monthly_reset_mode', 'rolling') = 'fixed'")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, "jsonb_typeof(extra->'quota_monthly_prorate') = 'boolean'")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, "NULLIF(extra->>'quota_monthly_prorated_until', '') ~")
	require.Contains(t, accountQuotaMonthlyEffectiveLimitExpr, ">= 0")
	for _, daily := range []bool{false, true} {
		for _, weekly := range []bool{false, true} {
			for _, monthly := range []bool{false, true} {
				query := accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(daily, weekly, monthly)]
				require.Contains(t, query, accountQuotaMonthlyEffectiveLimitExpr)
			}
		}
	}
}

func TestIncrementUsageBillingAccountQuota_UsesStatementMatchingHints(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(true, false)]).
		WithArgs(2.5, int64(9)).
		WillReturnRows(sqlmock.NewRows(accountQuotaStateColumns).AddRow(12.5, 100.0, 2.5, 50.0, 0.0, 0.0, 0.0, 0.0))
	mock.ExpectCommit()

	state, err := incrementUsageBillingAccountQuota(ctx, tx, 9, 2.5, true, false)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.InDelta(t, 12.5, state.TotalUsed, 0.000001)
	require.InDelta(t, 2.5, state.DailyUsed, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIncrementUsageBillingAccountQuota_FallsBackToFullStatementWhenGuardMisses(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(false, false)]).
		WithArgs(2.5, int64(9)).
		WillReturnRows(sqlmock.NewRows(accountQuotaStateColumns))
	mock.ExpectQuery(accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(true, true, true)]).
		WithArgs(2.5, int64(9)).
		WillReturnRows(sqlmock.NewRows(accountQuotaStateColumns).AddRow(2.5, 0.0, 0.0, 0.0, 2.5, 20.0, 0.0, 0.0))
	mock.ExpectCommit()

	state, err := incrementUsageBillingAccountQuota(ctx, tx, 9, 2.5, false, false)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.InDelta(t, 2.5, state.WeeklyUsed, 0.000001)
	require.InDelta(t, 20.0, state.WeeklyLimit, 0.000001)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIncrementUsageBillingAccountQuota_ReturnsNotFoundWhenFullStatementMisses(t *testing.T) {
	for _, hints := range [][3]bool{{false, false, false}, {true, false, false}, {false, true, false}, {true, true, false}, {false, false, true}, {true, false, true}, {false, true, true}, {true, true, true}} {
		ctx := context.Background()
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		require.NoError(t, err)

		mock.ExpectBegin()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		mock.ExpectQuery(accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(hints[0], hints[1], hints[2])]).
			WithArgs(2.5, int64(9)).
			WillReturnRows(sqlmock.NewRows(accountQuotaStateColumns))
		if !(hints[0] && hints[1] && hints[2]) {
			mock.ExpectQuery(accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(true, true, true)]).
				WithArgs(2.5, int64(9)).
				WillReturnRows(sqlmock.NewRows(accountQuotaStateColumns))
		}
		mock.ExpectRollback()

		state, err := incrementUsageBillingAccountQuota(ctx, tx, 9, 2.5, hints[0], hints[1], hints[2])
		require.ErrorIs(t, err, service.ErrAccountNotFound, "hints=%v", hints)
		require.Nil(t, state)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet(), "hints=%v", hints)
		_ = db.Close()
	}
}

func TestIncrementUsageBillingAccountQuota_MonthlyCrossingOutbox(t *testing.T) {
	for _, used := range []float64{9, 10, 12.5, 13} {
		t.Run(fmt.Sprint(used), func(t *testing.T) {
			ctx := context.Background()
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectBegin()
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			mock.ExpectQuery(accountQuotaIncrementSQLs[accountQuotaIncrementSQLIndex(false, false, true)]).
				WithArgs(2.5, int64(9)).
				WillReturnRows(sqlmock.NewRows(accountQuotaStateColumns).AddRow(100, 0, 0, 0, 0, 0, used, 10))
			if used >= 10 && used-2.5 < 10 {
				mock.ExpectExec(`INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)
                    VALUES ($1, $2, $3, $4, $5)
                    ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING`).
					WithArgs(service.SchedulerOutboxEventAccountChanged, int64(9), nil, nil, sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}
			mock.ExpectCommit()
			state, err := incrementUsageBillingAccountQuota(ctx, tx, 9, 2.5, false, false, true)
			require.NoError(t, err)
			require.Equal(t, used, state.MonthlyUsed)
			require.NoError(t, tx.Commit())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFilterSchedulerExtra_MonthlyQuotaContract(t *testing.T) {
	extra := map[string]any{
		"quota_monthly_limit": 50.0, "quota_monthly_used": 49.0,
		"quota_monthly_start": "2026-01-01T00:00:00Z", "quota_monthly_reset_mode": "fixed",
		"quota_monthly_reset_day": 31, "quota_monthly_reset_hour": 2,
		"quota_monthly_reset_at": "2026-02-28T02:00:00Z", "quota_reset_timezone": "UTC",
		"quota_monthly_prorate": true, "quota_monthly_prorated_limit": 25.0,
		"quota_monthly_prorated_until": "2026-02-28T02:00:00Z",
		"quota_daily_reset_at": "daily", "quota_weekly_reset_at": "weekly",
	}
	require.Equal(t, extra, filterSchedulerExtra(extra))
}
