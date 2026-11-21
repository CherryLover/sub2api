package repository

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelLimitRecoveryRequiresExactSnapshotAndAtomicOutbox(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		exec := &recordingSQLExecutor{result: rowsAffectedResult(affected)}
		repo := newAccountRepositoryWithSQL(nil, exec, nil)
		observed := map[string]any{"reason": "anthropic_fable_window", "rate_limit_reset_at": "2026-11-23T12:00:00Z"}
		applied, err := repo.ClearModelRateLimitSnapshotIfUnchanged(context.Background(), 42, "claude-fable-5", observed)
		require.NoError(t, err)
		require.Equal(t, affected == 1, applied)
		require.Len(t, exec.execQueries, 1)
		query := normalizeSQLWhitespace(exec.execQueries[0])
		require.Contains(t, query, "WITH updated AS (")
		require.Contains(t, query, "#- ARRAY['model_rate_limits', $1::text]")
		require.Contains(t, query, "#> ARRAY['model_rate_limits', $1::text] = $3::jsonb")
		require.Contains(t, query, "a.deleted_at IS NULL AND a.platform = $4")
		require.Contains(t, query, "INSERT INTO scheduler_outbox")
		require.Equal(t, "claude-fable-5", exec.execArgs[0][0])
		encoded, ok := exec.execArgs[0][2].(string)
		require.True(t, ok)
		require.JSONEq(t, `{"reason":"anthropic_fable_window","rate_limit_reset_at":"2026-11-23T12:00:00Z"}`, encoded)
		require.Equal(t, service.PlatformAnthropic, exec.execArgs[0][3])
		require.Equal(t, service.SchedulerOutboxEventAccountChanged, exec.execArgs[0][4])
	}
}

func TestModelLimitRecoveryFailsClosed(t *testing.T) {
	dbErr := errors.New("outbox insert failed")
	exec := &recordingSQLExecutor{err: dbErr}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	applied, err := repo.ClearModelRateLimitSnapshotIfUnchanged(context.Background(), 42, "claude-fable-5", map[string]any{"reason": "quota"})
	require.ErrorIs(t, err, dbErr)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	exec.execQueries = nil
	applied, err = repo.ClearModelRateLimitSnapshotIfUnchanged(context.Background(), 42, "claude-fable-5", map[string]any{"invalid": math.NaN()})
	require.Error(t, err)
	require.False(t, applied)
	require.Empty(t, exec.execQueries)
	applied, err = repo.ClearModelRateLimitSnapshotIfUnchanged(context.Background(), 42, "claude-fable-5", nil)
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, exec.execQueries)
}
