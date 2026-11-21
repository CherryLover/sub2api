//go:build integration

package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

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
	_, err = s.client.Account.UpdateOneID(child.ID).SetParentAccountID(parent.ID).Save(s.ctx)
	s.Require().NoError(err)
	applied, err := s.repo.UpdateOpenAIOAuthCredentialsIfUnchanged(s.ctx, child.ID, observed.Credentials, observed.ProxyID, map[string]any{"access_token": "incorrect-access", "refresh_token": "incorrect-refresh"})
	s.Require().NoError(err)
	s.Require().False(applied)
	stored, err := s.client.Account.Get(s.ctx, child.ID)
	s.Require().NoError(err)
	s.Require().Equal("child-refresh", stored.Credentials["refresh_token"])
}

func (s *AccountRepoSuite) TestUpdateOpenAIOAuthCredentialsIfUnchanged_OutboxFailureRollsBackRotation() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "openai-cas-rollback", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "original-access", "refresh_token": "original-refresh"}})
	observed, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	realSQL := s.repo.sql
	_, err = realSQL.ExecContext(s.ctx, "SAVEPOINT openai_rotation_atomicity")
	s.Require().NoError(err)
	s.repo.sql = &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: realSQL}
	applied, updateErr := s.repo.UpdateOpenAIOAuthCredentialsIfUnchanged(s.ctx, account.ID, observed.Credentials, observed.ProxyID, map[string]any{"access_token": "new-access", "refresh_token": "new-refresh"})
	s.repo.sql = realSQL
	_, err = realSQL.ExecContext(s.ctx, "ROLLBACK TO SAVEPOINT openai_rotation_atomicity")
	s.Require().NoError(err)
	s.Require().Error(updateErr)
	s.Require().False(applied)
	stored, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("original-refresh", stored.GetOpenAIRefreshToken())
}
