package service

import (
	"bytes"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"
	"time"
)

const openAIProbeReauthorizationRequired = "OpenAI OAuth credentials require reauthorization"

// Both the tested row and its credential owner are fenced. Shadow probes must
// never clear a newer parent's authentication state or store credentials locally.
type OpenAIAccountProbeStateRepository interface {
	SetOpenAIOAuthProbeStateIfUnchanged(context.Context, *Account, *Account, string) (bool, error)
}
type openAIAccountProbeProof struct{ account, owner *Account }
type openAIAccountProbeProofKey struct{}
type openAIAccountProbeModeKey struct{}

func plainOpenAIOAuthProbe(account *Account) bool {
	return account != nil && account.IsOpenAIOAuth() && !account.IsOpenAIPersonalAccessToken() && !account.IsOpenAIAgentIdentity()
}
func terminalOpenAIProbe401(body []byte) bool {
	switch strings.ToLower(extractUpstreamErrorCode(body)) {
	case "token_revoked", "token_invalidated", "insufficient_scope":
		return true
	}
	return strings.Contains(strings.ToLower(extractUpstreamErrorMessage(body)), "missing scopes:")
}
func retryableOpenAIProbe401(body []byte) bool {
	if !gjson.ValidBytes(body) || terminalOpenAIProbe401(body) || gjson.GetBytes(body, "detail").String() == "Unauthorized" {
		return false
	}
	switch strings.ToLower(extractUpstreamErrorCode(body)) {
	case "invalid_api_key", "token_expired", "expired_token", "authentication_error":
		return true
	}
	return false
}
func recoverableOpenAIProbeError(account *Account) bool {
	if !plainOpenAIOAuthProbe(account) || account.Status != StatusError {
		return false
	}
	const prefix = "Authentication failed (401):"
	message := strings.TrimSpace(account.ErrorMessage)
	return strings.HasPrefix(message, prefix) && retryableOpenAIProbe401([]byte(strings.TrimSpace(strings.TrimPrefix(message, prefix))))
}
func sameOpenAIProbeOwner(account, expected *Account) bool {
	if !plainOpenAIOAuthProbe(account) || expected == nil || account.ID != expected.ID || account.IsCredentialShadow() ||
		(!account.IsActive() && !recoverableOpenAIProbeError(account)) || strings.TrimSpace(account.GetOpenAIRefreshToken()) == "" {
		return false
	}
	if (account.ProxyID == nil) != (expected.ProxyID == nil) {
		return false
	}
	return account.ProxyID == nil || *account.ProxyID == *expected.ProxyID
}

type openAIProbeRefreshExecutor struct {
	OAuthRefreshExecutor
	expected *Account
	rejected string
}

func (e openAIProbeRefreshExecutor) CanRefresh(a *Account) bool {
	return sameOpenAIProbeOwner(a, e.expected) && e.OAuthRefreshExecutor.CanRefresh(a)
}
func (e openAIProbeRefreshExecutor) NeedsRefresh(a *Account, _ time.Duration) bool {
	return a.GetOpenAIAccessToken() == e.rejected
}
func (p *OpenAITokenProvider) refreshOpenAIProbeToken(ctx context.Context, owner *Account, rejected string) (*Account, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if p == nil || p.accountRepo == nil || p.refreshAPI == nil || p.executor == nil || rejected == "" || !sameOpenAIProbeOwner(owner, owner) {
		return nil, errors.New("OpenAI OAuth probe recovery is unavailable")
	}
	expected := snapshotOAuthRefreshAccount(owner)
	executor := openAIProbeRefreshExecutor{OAuthRefreshExecutor: p.executor, expected: expected, rejected: rejected}
	modeCtx := context.WithValue(withOAuthRefreshRequestPath(ctx), openAIAccountProbeModeKey{}, true)
	result, err := p.refreshAPI.RefreshIfNeeded(modeCtx, owner, executor, 0)
	if err != nil {
		return nil, err
	}
	fresh := result.Account
	if result.LockHeld {
		for attempt := 0; attempt < openAILockMaxAttempts; attempt++ {
			fresh, err = p.accountRepo.GetByID(ctx, expected.ID)
			if err != nil {
				return nil, err
			}
			if !sameOpenAIProbeOwner(fresh, expected) {
				return nil, errors.New("OpenAI probe account changed while refreshing")
			}
			if fresh.GetOpenAIAccessToken() != rejected {
				break
			}
			timer := time.NewTimer(openAILockInitialWait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !sameOpenAIProbeOwner(fresh, expected) || fresh.GetOpenAIAccessToken() == "" || fresh.GetOpenAIAccessToken() == rejected {
		return nil, errors.New("OpenAI probe did not obtain a usable replacement token")
	}
	if p.tokenCache != nil {
		if err := p.tokenCache.DeleteAccessToken(ctx, OpenAITokenCacheKey(expected)); err != nil {
			return nil, errors.New("OpenAI probe token cache invalidation failed")
		}
	}
	return snapshotOAuthRefreshAccount(fresh), nil
}
func (s *AccountTestService) doOpenAIAccountProbe(c *gin.Context, req *http.Request, proxyURL string, account, owner *Account, useTLS bool) (*http.Response, error) {
	doOnce := func(request *http.Request) (*http.Response, error) {
		if useTLS {
			return s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
		}
		return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	}
	if !plainOpenAIOAuthProbe(owner) {
		return doOnce(req)
	}
	attempted, credentialOwner := snapshotOAuthRefreshAccount(account), snapshotOAuthRefreshAccount(owner)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), openAIAccountProbeProofKey{}, (*openAIAccountProbeProof)(nil)))
	request := req
	for attempt := 0; attempt < 2; attempt++ {
		response, err := doOnce(request)
		if err != nil {
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			return nil, err
		}
		if response == nil || response.Body == nil {
			return nil, errors.New("OpenAI account probe returned no response body")
		}
		actualToken := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		tokenMatches := actualToken != "" && actualToken == credentialOwner.GetOpenAIAccessToken()
		if response.StatusCode != http.StatusUnauthorized {
			if response.StatusCode == http.StatusOK && tokenMatches && request.Context().Err() == nil {
				if attempted.ID == credentialOwner.ID {
					attempted = snapshotOAuthRefreshAccount(credentialOwner)
				}
				proof := &openAIAccountProbeProof{account: attempted, owner: credentialOwner}
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), openAIAccountProbeProofKey{}, proof))
			}
			return response, nil
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
		_ = response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		if readErr != nil || len(body) > 2<<20 || !tokenMatches {
			return response, nil
		}
		if attempt == 0 && retryableOpenAIProbe401(body) && request.GetBody != nil && sameOpenAIProbeOwner(credentialOwner, credentialOwner) {
			fresh, refreshErr := s.openAITokenProvider.refreshOpenAIProbeToken(request.Context(), credentialOwner, actualToken)
			if refreshErr == nil {
				retry := request.Clone(request.Context())
				retry.Body, err = request.GetBody()
				if err != nil {
					return response, nil
				}
				retry.Header.Set("Authorization", "Bearer "+fresh.GetOpenAIAccessToken())
				setOpenAIChatGPTAccountHeaders(retry.Header, fresh)
				_ = response.Body.Close()
				request = retry
				credentialOwner = fresh
				continue
			}
		}
		// An ambiguous early 401 remains probe failure, never permanent eviction.
		if (credentialOwner.IsActive() || recoverableOpenAIProbeError(credentialOwner)) && (terminalOpenAIProbe401(body) || strings.TrimSpace(credentialOwner.GetOpenAIRefreshToken()) == "") {
			if repo, ok := s.accountRepo.(OpenAIAccountProbeStateRepository); ok {
				_, _ = repo.SetOpenAIOAuthProbeStateIfUnchanged(request.Context(), credentialOwner, credentialOwner, openAIProbeReauthorizationRequired)
			}
		}
		return response, nil
	}
	return nil, errors.New("OpenAI account probe retry budget exhausted")
}
