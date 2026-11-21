//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// An actual held row lock is necessary here: a sequential stale-snapshot test
// cannot distinguish a locking owner check from an MVCC-only EXISTS clause.
func TestOpenAIProbeShadowCASRechecksOwnerAfterConcurrentReauthorization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "probe-lock-owner", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Credentials: map[string]any{"access_token": "observed", "refresh_token": "observed-refresh"},
	})
	child := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "probe-lock-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusError, ErrorMessage: "old child probe", ParentAccountID: &owner.ID, QuotaDimension: "spark",
	})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		for _, id := range []int64{child.ID, owner.ID} {
			if _, err := integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id=$1", id); err != nil {
				t.Errorf("cleanup outbox: %v", err)
			}
			if err := integrationEntClient.Account.DeleteOneID(id).Exec(cleanupCtx); err != nil {
				t.Errorf("cleanup account: %v", err)
			}
		}
	})
	repo := NewAccountRepository(integrationEntClient, integrationDB, nil).(*accountRepository)
	expectedOwner, err := repo.GetByID(ctx, owner.ID)
	require.NoError(t, err)
	expectedChild, err := repo.GetByID(ctx, child.ID)
	require.NoError(t, err)
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE accounts SET credentials=$1::jsonb WHERE id=$2`,
		`{"access_token":"reauthorized","refresh_token":"reauthorized-refresh"}`, owner.ID)
	require.NoError(t, err)
	conn, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	var pid int
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	probeRepo := newAccountRepositoryWithSQL(integrationEntClient, conn, nil)
	type outcome struct {
		applied bool
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		applied, err := probeRepo.SetOpenAIOAuthProbeStateIfUnchanged(ctx, expectedChild, expectedOwner, "")
		done <- outcome{applied, err}
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := integrationDB.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1)) > 0", pid).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 10*time.Millisecond, "probe must lock the credential owner before changing the shadow")
	require.NoError(t, tx.Commit())
	result := <-done
	require.NoError(t, result.err)
	require.False(t, result.applied)
	saved, err := repo.GetByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusError, saved.Status)
	require.Equal(t, "old child probe", saved.ErrorMessage)
	storedOwner, err := repo.GetByID(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, "reauthorized-refresh", storedOwner.GetOpenAIRefreshToken())
}
