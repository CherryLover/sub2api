package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type sessionLimitReleaseCacheStub struct {
	SessionLimitCache

	unregistered map[int64][]string
	err          error
}

func newSessionLimitReleaseCacheStub() *sessionLimitReleaseCacheStub {
	return &sessionLimitReleaseCacheStub{
		unregistered: make(map[int64][]string),
	}
}

func (s *sessionLimitReleaseCacheStub) UnregisterSession(_ context.Context, accountID int64, sessionUUID string) error {
	if s.err != nil {
		return s.err
	}
	s.unregistered[accountID] = append(s.unregistered[accountID], sessionUUID)
	return nil
}

func newSessionLimitTestAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"max_sessions": 1},
	}
}

func TestReleaseAccountSession_ReleasesRegisteredSlot(t *testing.T) {
	cache := newSessionLimitReleaseCacheStub()
	svc := &GatewayService{sessionLimitCache: cache}
	acc := newSessionLimitTestAccount()

	svc.ReleaseAccountSession(context.Background(), acc, "session-hash-1")

	require.Equal(t, []string{"session-hash-1"}, cache.unregistered[42])
}

func TestReleaseAccountSession_NoOpForInapplicableAccounts(t *testing.T) {
	apiKeyAcc := &Account{
		ID:       43,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"max_sessions": 1},
	}
	noLimitAcc := &Account{
		ID:       44,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
	}
	enabledAcc := newSessionLimitTestAccount()

	cases := []struct {
		name      string
		account   *Account
		sessionID string
	}{
		{"api_key_account", apiKeyAcc, "session-hash"},
		{"max_sessions_disabled", noLimitAcc, "session-hash"},
		{"empty_session_id", enabledAcc, ""},
		{"nil_account", nil, "session-hash"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := newSessionLimitReleaseCacheStub()
			svc := &GatewayService{sessionLimitCache: cache}
			svc.ReleaseAccountSession(context.Background(), tc.account, tc.sessionID)
			require.Empty(t, cache.unregistered)
		})
	}
}

func TestReleaseAccountSession_NilCacheAndErrorTolerance(t *testing.T) {
	svc := &GatewayService{}
	svc.ReleaseAccountSession(context.Background(), newSessionLimitTestAccount(), "session-hash")

	cache := &sessionLimitReleaseCacheStub{err: errors.New("redis down")}
	svc = &GatewayService{sessionLimitCache: cache}
	svc.ReleaseAccountSession(context.Background(), newSessionLimitTestAccount(), "session-hash")
}

func TestReleaseAccountSession_Idempotent(t *testing.T) {
	cache := newSessionLimitReleaseCacheStub()
	svc := &GatewayService{sessionLimitCache: cache}
	acc := newSessionLimitTestAccount()

	svc.ReleaseAccountSession(context.Background(), acc, "session-hash")
	svc.ReleaseAccountSession(context.Background(), acc, "session-hash")

	require.Len(t, cache.unregistered[42], 2)
}
