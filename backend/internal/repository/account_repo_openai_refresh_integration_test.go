//go:build integration

package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *AccountRepoSuite) TestUpdateOpenAIOAuthCredentialsIfUnchanged_AppliesAndPublishesSchedulerState() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "openai-refresh-success-cas-applied",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(10),
		},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err = s.repo.sql.ExecContext(s.ctx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
	s.Require().NoError(err)

	applied, err := s.repo.UpdateOpenAIOAuthCredentialsIfUnchanged(
		s.ctx,
		account.ID,
		observed.Credentials,
		observed.ProxyID,
		map[string]any{
			"access_token":   "rotated-access",
			"refresh_token":  "rotated-refresh",
			"_token_version": int64(11),
		},
	)

	s.Require().NoError(err)
	s.Require().True(applied)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("rotated-refresh", got.GetOpenAIRefreshToken())
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal("rotated-refresh", cacheRecorder.setAccounts[0].GetOpenAIRefreshToken())
	s.Require().NoError(cacheRecorder.setCtxErr)

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Equal(1, outboxCount)
}

func (s *AccountRepoSuite) TestUpdateOpenAIOAuthCredentialsIfUnchanged_SkipsConcurrentReauthorization() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "openai-refresh-success-cas-reauthorized",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":   "attempted-access",
			"refresh_token":  "attempted-refresh",
			"_token_version": int64(20),
		},
	})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, account.ID, map[string]any{
		"access_token":   "reauthorized-access",
		"refresh_token":  "reauthorized-refresh",
		"_token_version": int64(21),
	}))
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err = s.repo.sql.ExecContext(s.ctx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
	s.Require().NoError(err)

	applied, err := s.repo.UpdateOpenAIOAuthCredentialsIfUnchanged(
		s.ctx,
		account.ID,
		observed.Credentials,
		observed.ProxyID,
		map[string]any{
			"access_token":   "provider-access",
			"refresh_token":  "provider-refresh",
			"_token_version": int64(22),
		},
	)

	s.Require().NoError(err)
	s.Require().False(applied)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("reauthorized-refresh", got.GetOpenAIRefreshToken())
	s.Require().Empty(cacheRecorder.setAccounts)

	var outboxCount int
	err = scanSingleRow(
		s.ctx,
		s.repo.sql,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1 AND account_id = $2",
		[]any{service.SchedulerOutboxEventAccountChanged, account.ID},
		&outboxCount,
	)
	s.Require().NoError(err)
	s.Require().Zero(outboxCount)
}

func (s *AccountRepoSuite) TestUpdateOpenAIOAuthCredentialsIfUnchanged_DoesNotRotateCredentialShadow() {
	parent := mustCreateAccount(s.T(), s.client, &service.Account{Name: "openai-cas-parent", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"refresh_token": "parent-refresh"}})
	child := mustCreateAccount(s.T(), s.client, &service.Account{Name: "openai-cas-child", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "child-access", "refresh_token": "child-refresh"}})
	observed, err := s.repo.GetByID(s.ctx, child.ID)
	s.Require().NoError(err)
	_, err = s.client.Account.UpdateOneID(child.ID).SetParentAccountID(parent.ID).SetQuotaDimension("spark").Save(s.ctx)
	s.Require().NoError(err)
	applied, err := s.repo.UpdateOpenAIOAuthCredentialsIfUnchanged(s.ctx, child.ID, observed.Credentials, observed.ProxyID, map[string]any{"access_token": "incorrect-access", "refresh_token": "incorrect-refresh"})
	s.Require().NoError(err)
	s.Require().False(applied)
	stored, err := s.client.Account.Get(s.ctx, child.ID)
	s.Require().NoError(err)
	s.Require().Equal("child-refresh", stored.Credentials["refresh_token"])
}

func (s *AccountRepoSuite) TestUpdateOpenAIOAuthCredentialsIfUnchanged_OutboxFailureRollsBackRotation() {
	// Use an autocommit connection: rolling the fixture transaction back to a
	// savepoint would hide an accidental credential UPDATE outside the outbox statement.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	account := mustCreateAccount(s.T(), integrationEntClient, &service.Account{
		Name: "openai-cas-rollback", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "original-access", "refresh_token": "original-refresh"},
	})
	t := s.T()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID); err != nil {
			t.Errorf("remove OAuth rotation outbox fixture: %v", err)
		}
		if err := integrationEntClient.Account.DeleteOneID(account.ID).Exec(cleanupCtx); err != nil {
			t.Errorf("remove OAuth rotation account fixture: %v", err)
		}
	})
	repo := NewAccountRepository(integrationEntClient, integrationDB, nil).(*accountRepository)
	observed, err := repo.GetByID(ctx, account.ID)
	s.Require().NoError(err)
	repo.sql = &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: integrationDB}
	applied, updateErr := repo.UpdateOpenAIOAuthCredentialsIfUnchanged(ctx, account.ID, observed.Credentials, observed.ProxyID,
		map[string]any{"access_token": "new-access", "refresh_token": "new-refresh"})
	repo.sql = integrationDB
	s.Require().Error(updateErr)
	s.Require().False(applied)
	stored, err := repo.GetByID(ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("original-refresh", stored.GetOpenAIRefreshToken())
	var outboxCount int
	err = integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", account.ID).Scan(&outboxCount)
	s.Require().NoError(err)
	s.Require().Zero(outboxCount)
}
