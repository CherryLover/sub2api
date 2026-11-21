//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type openAIProbeRepository struct {
	*refreshAPIAccountRepo
	legacyErrors, stateWrites int
}

func (r *openAIProbeRepository) SetError(_ context.Context, id int64, message string) error {
	r.legacyErrors++
	if r.account.ID == id {
		r.account.Status = StatusError
		r.account.ErrorMessage = message
	}
	return nil
}
func (r *openAIProbeRepository) SetOpenAIOAuthProbeStateIfUnchanged(ctx context.Context, expected, expectedOwner *Account, message string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	a := r.account
	if a == nil || a.ID != expected.ID || a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth || a.IsCredentialShadow() ||
		!reflect.DeepEqual(a.Credentials, expected.Credentials) || !reflect.DeepEqual(a.ProxyID, expected.ProxyID) ||
		a.Status != expected.Status || a.ErrorMessage != expected.ErrorMessage || a.Schedulable != expected.Schedulable {
		return false, nil
	}
	r.stateWrites++
	a.ErrorMessage = message
	a.Status = StatusError
	if message == "" {
		a.Status = StatusActive
	}
	return true, nil
}

type openAIProbeUpstream struct {
	HTTPUpstream
	do func(*http.Request) (*http.Response, error)
}

func (u *openAIProbeUpstream) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.do(r)
}
func (u *openAIProbeUpstream) DoWithTLS(r *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.do(r)
}
func newOpenAIProbeRepo() *openAIProbeRepository {
	return &openAIProbeRepository{refreshAPIAccountRepo: &refreshAPIAccountRepo{account: newOpenAIOAuthRecoveryAccount()}}
}
func TestOpenAIProbeTransient401DoesNotPermanentlyDisableAccount(t *testing.T) {
	repo := newOpenAIProbeRepo()
	account := snapshotOAuthRefreshAccount(repo.account)
	upstream := &openAIProbeUpstream{do: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_api_key","message":"token rejected"}}`))}, nil
	}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: rawChatCompletionsTestConfig()}
	c := newOpenAIRejectedFieldTestContext(nil)
	err := svc.testOpenAIAccountConnection(c, account, "gpt-5.4", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Zero(t, repo.legacyErrors)
	require.Equal(t, StatusActive, repo.account.Status)
}
func TestOpenAIProbeRevocationCannotOverwriteConcurrentReauthorization(t *testing.T) {
	repo := newOpenAIProbeRepo()
	account := snapshotOAuthRefreshAccount(repo.account)
	upstream := &openAIProbeUpstream{do: func(*http.Request) (*http.Response, error) {
		repo.account.Credentials = map[string]any{"access_token": "new-auth", "refresh_token": "new-refresh"}
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"token_revoked"}}`))}, nil
	}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: rawChatCompletionsTestConfig()}
	err := svc.testOpenAIAccountConnection(newOpenAIRejectedFieldTestContext(nil), account, "gpt-5.4", "", AccountTestModeDefault)
	require.Error(t, err)
	require.Zero(t, repo.legacyErrors)
	require.Zero(t, repo.stateWrites)
	require.Equal(t, StatusActive, repo.account.Status)
	require.Equal(t, "new-auth", repo.account.GetOpenAIAccessToken())
}

func (r *openAIProbeRepository) UpdateExtra(context.Context, int64, map[string]any) error { return nil }
func openAIProbeOK() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"probe_ok\"}}\n\n"))}
}
func TestOpenAIProbeRefreshesOnceAndRecoversOnlyProvenGeneration(t *testing.T) {
	for _, rejection := range []bool{false, true} {
		t.Run(fmt.Sprint(rejection), func(t *testing.T) {
			repo := newOpenAIProbeRepo()
			repo.account.Status = StatusError
			repo.account.ErrorMessage = `Authentication failed (401): {"error":{"code":"invalid_api_key"}}`
			until := time.Now().Add(time.Hour)
			repo.account.RateLimitResetAt = &until
			attempted := snapshotOAuthRefreshAccount(repo.account)
			executor := &refreshAPIExecutorStub{credentials: map[string]any{"access_token": "fresh-at", "refresh_token": "fresh-rt"}}
			provider := NewOpenAITokenProvider(repo, nil, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
			requests := 0
			var bodies []string
			upstream := &openAIProbeUpstream{do: func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				bodies = append(bodies, string(body))
				requests++
				if requests == 1 || rejection {
					return newOpenAIRejectedFieldTestResponse(401, `{"error":{"code":"invalid_api_key"}}`), nil
				}
				require.Equal(t, "Bearer fresh-at", req.Header.Get("Authorization"))
				return openAIProbeOK(), nil
			}}
			svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, openAITokenProvider: provider, cfg: rawChatCompletionsTestConfig()}
			c := newOpenAIRejectedFieldTestContext(nil)
			err := svc.testOpenAIAccountConnection(c, attempted, "gpt-5.4", "", AccountTestModeDefault)
			require.Equal(t, 2, requests)
			require.Equal(t, 1, executor.refreshCalls)
			require.Equal(t, bodies[0], bodies[1])
			require.Zero(t, repo.legacyErrors)
			if rejection {
				require.Error(t, err)
				require.Equal(t, StatusError, repo.account.Status)
				return
			}
			require.NoError(t, err)
			result, err := newRateLimitServiceForTest(repo).RecoverAccountAfterSuccessfulTest(c.Request.Context(), attempted.ID)
			require.NoError(t, err)
			require.True(t, result.ClearedError)
			require.False(t, result.ClearedRateLimit)
			require.Equal(t, StatusActive, repo.account.Status)
			require.Equal(t, &until, repo.account.RateLimitResetAt)
			require.Equal(t, "old-rt", attempted.GetOpenAIRefreshToken(), "probe snapshots must not mutate")
		})
	}
}
func TestOpenAIProbeSuccessCannotClearNewerState(t *testing.T) {
	for _, mutation := range []string{"credentials", "status", "error", "scheduler", "no proof", "revoked"} {
		t.Run(mutation, func(t *testing.T) {
			repo := newOpenAIProbeRepo()
			repo.account.Status = StatusError
			repo.account.ErrorMessage = `Authentication failed (401): {"error":{"code":"token_expired"}}`
			expected := snapshotOAuthRefreshAccount(repo.account)
			ctx := context.WithValue(t.Context(), openAIAccountProbeProofKey{}, &openAIAccountProbeProof{account: expected, owner: expected})
			switch mutation {
			case "credentials":
				repo.account.Credentials = map[string]any{"access_token": "reauth", "refresh_token": "reauth-rt"}
			case "status":
				repo.account.Status = StatusDisabled
			case "error":
				repo.account.ErrorMessage = "administrator blocked this account"
			case "scheduler":
				repo.account.Schedulable = !repo.account.Schedulable
			case "no proof":
				ctx = t.Context()
			case "revoked":
				repo.account.ErrorMessage = openAIProbeReauthorizationRequired
				expected.ErrorMessage = openAIProbeReauthorizationRequired
			}
			result, err := newRateLimitServiceForTest(repo).RecoverAccountAfterSuccessfulTest(ctx, repo.account.ID)
			require.NoError(t, err)
			require.False(t, result.ClearedError)
			require.Zero(t, repo.stateWrites)
		})
	}
}
func TestOpenAIProbeRecoveryFailsClosedOnLockError(t *testing.T) {
	repo := newOpenAIProbeRepo()
	executor := &refreshAPIExecutorStub{credentials: map[string]any{"access_token": "fresh", "refresh_token": "fresh-rt"}}
	cache := &refreshAPICacheStub{lockErr: errors.New("redis unavailable")}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
	_, err := provider.refreshOpenAIProbeToken(t.Context(), repo.account, repo.account.GetOpenAIAccessToken())
	require.Error(t, err)
	require.Zero(t, executor.refreshCalls)
}
func TestOpenAIProbeDoesNotRefreshRevokedPATOrOverride(t *testing.T) {
	for _, kind := range []string{"revoked", "PAT", "override", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			repo := newOpenAIProbeRepo()
			account := snapshotOAuthRefreshAccount(repo.account)
			if kind == "PAT" {
				account.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken
			}
			executor := &refreshAPIExecutorStub{credentials: map[string]any{"access_token": "fresh", "refresh_token": "fresh-rt"}}
			provider := NewOpenAITokenProvider(repo, nil, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
			body := `{"error":{"code":"invalid_api_key"}}`
			if kind == "revoked" {
				body = `{"error":{"code":"token_revoked"}}`
			}
			if kind == "unknown" {
				body = `{"error":{"message":"unclassified auth failure"}}`
			}
			calls := 0
			upstream := &openAIProbeUpstream{do: func(*http.Request) (*http.Response, error) {
				calls++
				return newOpenAIRejectedFieldTestResponse(401, body), nil
			}}
			svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, openAITokenProvider: provider, cfg: rawChatCompletionsTestConfig()}
			c := newOpenAIRejectedFieldTestContext(nil)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", bytes.NewReader([]byte(`{"input":"hello"}`)))
			require.NoError(t, err)
			token := account.GetOpenAIAccessToken()
			if kind == "override" {
				token = "operator-override"
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := svc.doOpenAIAccountProbe(c, req, "", account, account, false)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, 1, calls)
			require.Zero(t, executor.refreshCalls)
			if kind == "revoked" {
				require.Equal(t, 1, repo.stateWrites)
				require.Equal(t, openAIProbeReauthorizationRequired, repo.account.ErrorMessage)
			} else {
				require.Zero(t, repo.stateWrites)
			}
		})
	}
}
func TestOpenAIBackgroundProbeCarriesNonSerializedRecoveryProof(t *testing.T) {
	repo := newOpenAIProbeRepo()
	repo.account.Status = StatusError
	repo.account.ErrorMessage = `Authentication failed (401): {"error":{"code":"token_expired"}}`
	svc := &AccountTestService{accountRepo: repo, httpUpstream: &openAIProbeUpstream{do: func(*http.Request) (*http.Response, error) { return openAIProbeOK(), nil }}, cfg: rawChatCompletionsTestConfig()}
	result, err := svc.RunTestBackground(t.Context(), repo.account.ID, "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "success", result.Status)
	require.NotNil(t, result.openAIProbeProof)
}

func TestOpenAIProbeTerminal401PreservesExplicitDisabledAndAdminErrors(t *testing.T) {
	for _, tc := range []struct{ status, message string }{{StatusDisabled, "manually disabled"}, {StatusError, "operator blocked account"}, {StatusError, openAIProbeReauthorizationRequired}} {
		repo := newOpenAIProbeRepo()
		repo.account.Status = tc.status
		repo.account.ErrorMessage = tc.message
		account := snapshotOAuthRefreshAccount(repo.account)
		upstream := &openAIProbeUpstream{do: func(*http.Request) (*http.Response, error) {
			return newOpenAIRejectedFieldTestResponse(401, `{"error":{"code":"token_revoked"}}`), nil
		}}
		svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: rawChatCompletionsTestConfig()}
		err := svc.testOpenAIAccountConnection(newOpenAIRejectedFieldTestContext(nil), account, "gpt-5.4", "", AccountTestModeDefault)
		require.Error(t, err)
		require.Zero(t, repo.stateWrites)
		require.Zero(t, repo.legacyErrors)
		require.Equal(t, tc.status, repo.account.Status)
		require.Equal(t, tc.message, repo.account.ErrorMessage)
	}
}
func TestOpenAIProbeProofNeverSerializesCredentials(t *testing.T) {
	account := newOpenAIOAuthRecoveryAccount()
	result := ScheduledTestResult{Status: "success", openAIProbeProof: &openAIAccountProbeProof{account: account, owner: account}}
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "rejected-at")
	require.NotContains(t, string(encoded), "old-rt")
	require.NotContains(t, string(encoded), "openAIProbeProof")
}
