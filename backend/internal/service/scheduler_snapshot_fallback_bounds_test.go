package service

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerFallbackSharedLoadStopsWithService(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		repo := &coalesceFollowupRepo{load: func(ctx context.Context, _ int32) ([]Account, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		svc := NewSchedulerSnapshotService(nil, nil, repo, nil, coalesceFollowupConfig())
		result := make(chan error, 1)
		go func() {
			_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
			result <- err
		}()
		<-entered
		svc.Stop()
		require.ErrorIs(t, <-result, context.Canceled)
		_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, int32(1), repo.calls.Load())
	})
}

func TestSchedulerFallbackSharedLoadHasDefaultDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := coalesceFollowupConfig()
		cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds = 0
		repo := &coalesceFollowupRepo{load: func(ctx context.Context, _ int32) ([]Account, error) { <-ctx.Done(); return nil, ctx.Err() }}
		svc := NewSchedulerSnapshotService(nil, nil, repo, nil, cfg)
		before := time.Now()
		_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, schedulerSharedFallbackTimeout, time.Since(before))
	})
}

func TestSchedulerFallbackSharedLoadPanicIsNotCached(t *testing.T) {
	repo := &coalesceFollowupRepo{load: func(_ context.Context, n int32) ([]Account, error) {
		if n == 1 {
			panic("repository panic")
		}
		return coalesceFollowupAccounts(2), nil
	}}
	svc := NewSchedulerSnapshotService(nil, nil, repo, nil, coalesceFollowupConfig())
	_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
	require.EqualError(t, err, "scheduler shared database fallback panicked")
	accounts, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(2), accounts[0].ID)
}

type unavailableEpochFollowupCache struct{ coalesceFollowupCache }

func (c *unavailableEpochFollowupCache) CaptureBucketWriteToken(context.Context, SchedulerBucket) (SchedulerBucketWriteToken, error) {
	return SchedulerBucketWriteToken{}, errors.New("lifecycle unavailable")
}

func TestSchedulerFallbackUnknownEpochDoesNotShareOrDetach(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		repo := &coalesceFollowupRepo{load: func(ctx context.Context, n int32) ([]Account, error) {
			select {
			case <-release:
				return coalesceFollowupAccounts(int64(n)), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}
		cache := &unavailableEpochFollowupCache{}
		svc := NewSchedulerSnapshotService(cache, nil, repo, nil, coalesceFollowupConfig())
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		leader, follower := make(chan error, 1), make(chan error, 1)
		go func() { _, _, err := svc.ListSchedulableAccounts(ctx, nil, PlatformOpenAI, false); leader <- err }()
		synctest.Wait()
		go func() {
			_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
			follower <- err
		}()
		synctest.Wait()
		require.Equal(t, int32(2), repo.calls.Load())
		cancel()
		require.ErrorIs(t, <-leader, context.Canceled)
		close(release)
		require.NoError(t, <-follower)
		require.Zero(t, cache.writes.Load())
	})
}

func TestSchedulerFallbackDisabledStillRejectsMiss(t *testing.T) {
	cfg := coalesceFollowupConfig()
	cfg.Gateway.Scheduling.DbFallbackEnabled = false
	repo := &coalesceFollowupRepo{load: func(context.Context, int32) ([]Account, error) {
		t.Error("disabled DB fallback was invoked")
		return nil, nil
	}}
	svc := NewSchedulerSnapshotService(nil, nil, repo, nil, cfg)
	_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Zero(t, repo.calls.Load())
}
