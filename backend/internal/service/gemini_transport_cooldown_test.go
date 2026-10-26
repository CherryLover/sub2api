package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiTransportCooldownRepo struct {
	AccountRepository
	calls   int
	reason  string
	account int64
	until   time.Time
	err     error
}

func (r *geminiTransportCooldownRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.calls++
	r.account = id
	r.reason = reason
	r.until = until
	return r.err
}

func TestGeminiTransportPersistentFailureTempUnschedulesWithoutDisablingAccount(t *testing.T) {
	repo := &geminiTransportCooldownRepo{}
	service := &GeminiMessagesCompatService{accountRepo: repo}
	account := &Account{ID: 42, Platform: PlatformGemini, Status: StatusActive}
	before := time.Now()
	service.tempUnscheduleGeminiTransportError(context.Background(), account, errors.New("dial tcp: connection refused"))
	require.Equal(t, 1, repo.calls)
	require.Equal(t, int64(42), repo.account)
	require.Contains(t, repo.reason, "upstream transport error")
	require.True(t, repo.until.After(before))
	require.NotNil(t, account.TempUnschedulableUntil)
	require.True(t, account.TempUnschedulableUntil.After(before))
	require.Contains(t, account.TempUnschedulableReason, "transport error")
	require.Equal(t, StatusActive, account.Status)
}

func TestGeminiTransportTransientAndCanceledDoNotCooldown(t *testing.T) {
	repo := &geminiTransportCooldownRepo{}
	service := &GeminiMessagesCompatService{accountRepo: repo}
	account := &Account{ID: 42, Platform: PlatformGemini, Status: StatusActive}
	service.tempUnscheduleGeminiTransportError(context.Background(), account, io.EOF)
	service.tempUnscheduleGeminiTransportError(context.Background(), account, context.Canceled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.tempUnscheduleGeminiTransportError(ctx, account, errors.New("connection refused"))
	require.Zero(t, repo.calls)
}

func TestGeminiTransportCooldownDatabaseFailureDoesNotRewriteAccountState(t *testing.T) {
	repo := &geminiTransportCooldownRepo{err: errors.New("database unavailable")}
	service := &GeminiMessagesCompatService{accountRepo: repo}
	oldUntil := time.Now().Add(20 * time.Minute)
	account := &Account{ID: 42, Platform: PlatformGemini, TempUnschedulableUntil: &oldUntil}
	service.tempUnscheduleGeminiTransportError(context.Background(), account, errors.New("proxy authentication required"))
	require.Equal(t, 1, repo.calls)
	require.Equal(t, oldUntil, *account.TempUnschedulableUntil, "existing in-memory cooldown must not be rolled back")
	require.True(t, strings.Contains(repo.reason, "proxy") || strings.Contains(repo.reason, "transport"))
}

func TestGeminiTransportCooldownDatabaseFailurePreservesNewMemoryPause(t *testing.T) {
	repo := &geminiTransportCooldownRepo{err: errors.New("db failed")}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	account := &Account{ID: 43, Platform: PlatformGemini, Status: StatusActive, Schedulable: true}
	svc.tempUnscheduleGeminiTransportError(context.Background(), account, errors.New("connection refused"))
	require.Equal(t, 1, repo.calls)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.False(t, account.IsSchedulable(), "memory cooldown still applies after failed DB write")
}
