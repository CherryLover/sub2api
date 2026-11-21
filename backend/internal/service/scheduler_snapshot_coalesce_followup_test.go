package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type coalesceFollowupRepo struct {
	AccountRepository
	calls atomic.Int32
	load  func(context.Context, int32) ([]Account, error)
}

func (r *coalesceFollowupRepo) ListModelAvailabilityCandidates(ctx context.Context, _ *int64, _ []string, _ bool) ([]Account, error) {
	n := r.calls.Add(1)
	return r.load(ctx, n)
}

type coalesceFollowupCache struct {
	SchedulerCache
	epoch  atomic.Int64
	writes atomic.Int32
}

func (c *coalesceFollowupCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return nil, false, nil
}
func (c *coalesceFollowupCache) CaptureBucketWriteToken(_ context.Context, b SchedulerBucket) (SchedulerBucketWriteToken, error) {
	return SchedulerBucketWriteToken{Bucket: b, Epoch: c.epoch.Load()}, nil
}
func (c *coalesceFollowupCache) SetSnapshot(_ context.Context, _ SchedulerBucket, token SchedulerBucketWriteToken, _ []Account) error {
	if token.Epoch != c.epoch.Load() {
		return ErrSchedulerBucketWriteFenced
	}
	c.writes.Add(1)
	return nil
}
func coalesceFollowupConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.DbFallbackEnabled = true
	cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds = 2
	return cfg
}
func coalesceFollowupAccounts(id int64) []Account {
	return []Account{{ID: id, Name: "shared", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1}}
}
func TestSchedulerFallbackCoalescesWithinBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &coalesceFollowupCache{}
		cache.epoch.Store(1)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		repo := &coalesceFollowupRepo{load: func(ctx context.Context, _ int32) ([]Account, error) {
			once.Do(func() { close(entered) })
			select {
			case <-release:
				return coalesceFollowupAccounts(1), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}
		cfg := coalesceFollowupConfig()
		cfg.Gateway.Scheduling.DbFallbackMaxQPS = 1
		svc := NewSchedulerSnapshotService(cache, nil, repo, nil, cfg)
		type result struct {
			accounts []Account
			err      error
		}
		results := make(chan result, 12)
		start := make(chan struct{})
		for i := 0; i < 12; i++ {
			go func() {
				<-start
				a, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
				results <- result{a, err}
			}()
		}
		close(start)
		<-entered
		synctest.Wait()
		close(release)
		var first []Account
		for i := 0; i < 12; i++ {
			r := <-results
			require.NoError(t, r.err)
			require.Len(t, r.accounts, 1)
			if first == nil {
				first = r.accounts
			} else {
				r.accounts[0].Name = "local"
				require.Equal(t, "shared", first[0].Name)
			}
		}
		require.Equal(t, int32(1), repo.calls.Load())
		require.Equal(t, int32(1), cache.writes.Load())
	})
}
func TestSchedulerFallbackCanceledLeaderDoesNotCancelWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &coalesceFollowupCache{}
		cache.epoch.Store(1)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		repo := &coalesceFollowupRepo{load: func(ctx context.Context, _ int32) ([]Account, error) {
			once.Do(func() { close(entered) })
			select {
			case <-release:
				return coalesceFollowupAccounts(1), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}
		svc := NewSchedulerSnapshotService(cache, nil, repo, nil, coalesceFollowupConfig())
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		leader := make(chan error, 1)
		go func() { _, _, err := svc.ListSchedulableAccounts(ctx, nil, PlatformOpenAI, false); leader <- err }()
		<-entered
		cancel()
		require.ErrorIs(t, <-leader, context.Canceled)
		follower := make(chan error, 1)
		go func() {
			_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
			follower <- err
		}()
		synctest.Wait()
		close(release)
		require.NoError(t, <-follower)
		require.Equal(t, int32(1), repo.calls.Load())
	})
}
func TestSchedulerFallbackLifecycleEpochCannotJoinStaleQuery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &coalesceFollowupCache{}
		cache.epoch.Store(1)
		entered := make(chan int32, 2)
		release := make(chan struct{})
		repo := &coalesceFollowupRepo{load: func(ctx context.Context, n int32) ([]Account, error) {
			entered <- n
			select {
			case <-release:
				return coalesceFollowupAccounts(int64(n)), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}
		svc := NewSchedulerSnapshotService(cache, nil, repo, nil, coalesceFollowupConfig())
		results := make(chan []Account, 2)
		go func() { a, _, _ := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false); results <- a }()
		require.Equal(t, int32(1), <-entered)
		cache.epoch.Store(2)
		go func() { a, _, _ := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false); results <- a }()
		select {
		case n := <-entered:
			require.Equal(t, int32(2), n)
		case <-time.After(time.Second):
			t.Error("new bucket epoch joined an old snapshot query")
		}
		close(release)
		require.Len(t, <-results, 1)
		require.Len(t, <-results, 1)
		require.Equal(t, int32(2), repo.calls.Load())
		require.Equal(t, int32(1), cache.writes.Load())
	})
}
func TestSchedulerFallbackDoesNotCacheErrors(t *testing.T) {
	expected := errors.New("database unavailable")
	repo := &coalesceFollowupRepo{load: func(_ context.Context, n int32) ([]Account, error) {
		if n == 1 {
			return nil, expected
		}
		return coalesceFollowupAccounts(1), nil
	}}
	svc := NewSchedulerSnapshotService(nil, nil, repo, nil, coalesceFollowupConfig())
	_, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
	require.ErrorIs(t, err, expected)
	accounts, _, err := svc.ListSchedulableAccounts(t.Context(), nil, PlatformOpenAI, false)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int32(2), repo.calls.Load())
}
func BenchmarkSchedulerFallbackConcurrentBucket(b *testing.B) {
	repo := &coalesceFollowupRepo{load: func(_ context.Context, _ int32) ([]Account, error) {
		time.Sleep(time.Millisecond)
		return coalesceFollowupAccounts(1), nil
	}}
	svc := NewSchedulerSnapshotService(nil, nil, repo, nil, coalesceFollowupConfig())
	b.SetParallelism(8)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, _, err := svc.ListSchedulableAccounts(context.Background(), nil, PlatformOpenAI, false); err != nil {
				b.Error(err)
			}
		}
	})
	b.ReportMetric(float64(repo.calls.Load())/float64(b.N), "db_queries/op")
}
