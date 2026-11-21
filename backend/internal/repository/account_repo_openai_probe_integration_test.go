//go:build integration

package repository

import (
	"context"
	"database/sql"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"strings"
	"time"
)

func (s *AccountRepoSuite) TestOpenAIProbeStateRestoresOnlyObservedCredentials() {
	a := mustCreateAccount(s.T(), s.client, &service.Account{Name: "openai-probe-generation", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, ErrorMessage: "old probe", Schedulable: true, Credentials: map[string]any{"access_token": "tested", "refresh_token": "tested-rt"}})
	observed, err := s.repo.GetByID(s.ctx, a.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, a.ID, map[string]any{"access_token": "reauth", "refresh_token": "reauth-rt"}))
	applied, err := s.repo.SetOpenAIOAuthProbeStateIfUnchanged(s.ctx, observed, observed, "")
	s.Require().NoError(err)
	s.Require().False(applied)
	current, err := s.repo.GetByID(s.ctx, a.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, current.Status)
	applied, err = s.repo.SetOpenAIOAuthProbeStateIfUnchanged(s.ctx, current, current, "")
	s.Require().NoError(err)
	s.Require().True(applied)
	saved, err := s.repo.GetByID(s.ctx, a.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, saved.Status)
	s.Require().Empty(saved.ErrorMessage)
	s.Require().Equal("reauth-rt", saved.GetOpenAIRefreshToken())
	s.Require().True(saved.Schedulable)
}
func (s *AccountRepoSuite) TestOpenAIProbeShadowRecoveryFencesParentReauthorization() {
	owner := mustCreateAccount(s.T(), s.client, &service.Account{Name: "probe-parent", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "tested", "refresh_token": "tested-rt"}})
	child := mustCreateAccount(s.T(), s.client, &service.Account{Name: "probe-shadow", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, ErrorMessage: "old child probe", ParentAccountID: &owner.ID, QuotaDimension: "spark"})
	expectedOwner, err := s.repo.GetByID(s.ctx, owner.ID)
	s.Require().NoError(err)
	expectedChild, err := s.repo.GetByID(s.ctx, child.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.repo.UpdateCredentials(s.ctx, owner.ID, map[string]any{"access_token": "new-owner", "refresh_token": "new-owner-rt"}))
	applied, err := s.repo.SetOpenAIOAuthProbeStateIfUnchanged(s.ctx, expectedChild, expectedOwner, "")
	s.Require().NoError(err)
	s.Require().False(applied)
	current, err := s.repo.GetByID(s.ctx, child.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, current.Status)
}

type failOpenAIProbeOutboxExecutor struct{ sqlExecutor }

func (e *failOpenAIProbeOutboxExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if strings.Contains(query, "WITH credential_owner AS") && strings.Contains(query, "INSERT INTO scheduler_outbox") {
		args = append([]any(nil), args...)
		args[len(args)-1] = nil
	}
	return e.sqlExecutor.ExecContext(ctx, query, args...)
}
func (s *AccountRepoSuite) TestOpenAIProbeOutboxFailureCannotPartiallyClearError() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := mustCreateAccount(s.T(), integrationEntClient, &service.Account{Name: "probe-atomic", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, ErrorMessage: "old probe", Credentials: map[string]any{"access_token": "tested", "refresh_token": "tested-rt"}})
	t := s.T()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := integrationDB.ExecContext(ctx, "DELETE FROM scheduler_outbox WHERE account_id=$1", a.ID); err != nil {
			t.Errorf("cleanup probe outbox: %v", err)
		}
		if err := integrationEntClient.Account.DeleteOneID(a.ID).Exec(ctx); err != nil {
			t.Errorf("cleanup probe account: %v", err)
		}
	})
	repo := NewAccountRepository(integrationEntClient, integrationDB, nil).(*accountRepository)
	observed, err := repo.GetByID(ctx, a.ID)
	s.Require().NoError(err)
	repo.sql = &failOpenAIProbeOutboxExecutor{sqlExecutor: integrationDB}
	applied, err := repo.SetOpenAIOAuthProbeStateIfUnchanged(ctx, observed, observed, "")
	s.Require().Error(err)
	s.Require().False(applied)
	repo.sql = integrationDB
	saved, err := repo.GetByID(ctx, a.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, saved.Status)
	s.Require().Equal("old probe", saved.ErrorMessage)
}

func (s *AccountRepoSuite) TestOpenAIProbeShadowPositiveCASPreservesCredentialOwnership() {
	owner := mustCreateAccount(s.T(), s.client, &service.Account{Name: "probe-positive-parent", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Credentials: map[string]any{"access_token": "tested", "refresh_token": "tested-rt"}})
	child := mustCreateAccount(s.T(), s.client, &service.Account{Name: "probe-positive-child", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, ErrorMessage: "old child probe", ParentAccountID: &owner.ID, QuotaDimension: "spark"})
	expectedOwner, err := s.repo.GetByID(s.ctx, owner.ID)
	s.Require().NoError(err)
	expectedChild, err := s.repo.GetByID(s.ctx, child.ID)
	s.Require().NoError(err)
	applied, err := s.repo.SetOpenAIOAuthProbeStateIfUnchanged(s.ctx, expectedChild, expectedOwner, "")
	s.Require().NoError(err)
	s.Require().True(applied)
	stored, err := s.client.Account.Get(s.ctx, child.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, stored.Status)
	s.Require().Empty(stored.Credentials)
	s.Require().Equal(owner.ID, *stored.ParentAccountID)
	actualOwner, err := s.repo.GetByID(s.ctx, owner.ID)
	s.Require().NoError(err)
	s.Require().Equal("tested-rt", actualOwner.GetOpenAIRefreshToken())
}
