package repository

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpenAIProbeStateCASLocksOwnerAndPublishesAtomically(t *testing.T) {
	for _, rows := range []int64{0, 1} {
		executor := &recordingSQLExecutor{result: rowsAffectedResult(rows)}
		repo := newAccountRepositoryWithSQL(nil, executor, nil)
		owner := &service.Account{ID: 23, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Credentials: map[string]any{"access_token": "synthetic", "refresh_token": "synthetic-rt"}}
		target := &service.Account{ID: 24, ParentAccountID: &owner.ID, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, ErrorMessage: "old probe", Schedulable: true}
		applied, err := repo.SetOpenAIOAuthProbeStateIfUnchanged(t.Context(), target, owner, "")
		require.NoError(t, err)
		require.Equal(t, rows == 1, applied)
		require.Len(t, executor.execQueries, 1)
		query := normalizeSQLWhitespace(executor.execQueries[0])
		for _, part := range []string{"FOR UPDATE", "o.credentials=$4::jsonb", "o.status=$6", "o.schedulable=$8", "a.parent_account_id IS NOT DISTINCT FROM $12", "a.credentials=$13::jsonb", "a.status=$15", "a.schedulable=$17", "INSERT INTO scheduler_outbox"} {
			require.Contains(t, query, part)
		}
		require.Len(t, executor.execArgs[0], 18)
		require.Equal(t, owner.ID, executor.execArgs[0][0])
		require.Equal(t, target.ID, executor.execArgs[0][10])
		require.Equal(t, service.SchedulerOutboxEventAccountChanged, executor.execArgs[0][17])
	}
}
func TestOpenAIProbeStateCASFailsClosed(t *testing.T) {
	dbErr := errors.New("outbox failure")
	executor := &recordingSQLExecutor{err: dbErr}
	repo := newAccountRepositoryWithSQL(nil, executor, nil)
	account := &service.Account{ID: 23, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	applied, err := repo.SetOpenAIOAuthProbeStateIfUnchanged(t.Context(), account, account, "reauthorize")
	require.ErrorIs(t, err, dbErr)
	require.False(t, applied)
	executor.execQueries = nil
	foreign := *account
	foreign.ID++
	applied, err = repo.SetOpenAIOAuthProbeStateIfUnchanged(t.Context(), account, &foreign, "")
	require.NoError(t, err)
	require.False(t, applied)
	require.Empty(t, executor.execQueries)
}
