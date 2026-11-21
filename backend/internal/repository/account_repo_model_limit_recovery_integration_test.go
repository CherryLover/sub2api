//go:build integration

package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *AccountRepoSuite) TestModelLimitRecoveryPreservesConcurrentReasonAndOtherScopes() {
	observed := map[string]any{"reason": "fable_window", "rate_limit_reset_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "fable-limit-fence", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
		Extra: map[string]any{"model_rate_limits": map[string]any{"claude-fable-5": observed, "other-model": map[string]any{"reason": "admin"}}}})
	replacement := map[string]any{"reason": "credits_required", "rate_limit_reset_at": observed["rate_limit_reset_at"]}
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"model_rate_limits": map[string]any{"claude-fable-5": replacement, "other-model": map[string]any{"reason": "admin"}}}))
	applied, err := s.repo.ClearModelRateLimitSnapshotIfUnchanged(s.ctx, account.ID, "claude-fable-5", observed)
	s.Require().NoError(err)
	s.Require().False(applied)
	stored, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	limits, ok := stored.Extra["model_rate_limits"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal(replacement, limits["claude-fable-5"])
	s.Require().Contains(limits, "other-model")
	applied, err = s.repo.ClearModelRateLimitSnapshotIfUnchanged(s.ctx, account.ID, "claude-fable-5", replacement)
	s.Require().NoError(err)
	s.Require().True(applied)
	stored, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	limits, ok = stored.Extra["model_rate_limits"].(map[string]any)
	s.Require().True(ok)
	s.Require().NotContains(limits, "claude-fable-5")
	s.Require().Contains(limits, "other-model")
}

func (s *AccountRepoSuite) TestModelLimitRecoveryOutboxFailureIsAtomic() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	observed := map[string]any{"reason": "fable_window", "rate_limit_reset_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	account := mustCreateAccount(s.T(), integrationEntClient, &service.Account{Name: "fable-recovery-atomic", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Extra: map[string]any{"model_rate_limits": map[string]any{"claude-fable-5": observed}}})
	t := s.T()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID); err != nil {
			t.Errorf("cleanup outbox: %v", err)
		}
		if err := integrationEntClient.Account.DeleteOneID(account.ID).Exec(cleanupCtx); err != nil {
			t.Errorf("cleanup account: %v", err)
		}
	})
	repo := NewAccountRepository(integrationEntClient, integrationDB, nil).(*accountRepository)
	repo.sql = &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: integrationDB}
	applied, err := repo.ClearModelRateLimitSnapshotIfUnchanged(ctx, account.ID, "claude-fable-5", observed)
	s.Require().Error(err)
	s.Require().False(applied)
	repo.sql = integrationDB
	stored, err := repo.GetByID(ctx, account.ID)
	s.Require().NoError(err)
	limits, ok := stored.Extra["model_rate_limits"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal(observed, limits["claude-fable-5"])
}
