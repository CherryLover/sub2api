package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"
)

const schedulerSharedFallbackTimeout = 30 * time.Second

// Coalescing only shares overlapping reads, never a second account cache.
// Each caller captures the lifecycle token before joining: a reopened bucket
// must never join a query from its retired epoch.
type schedulerFallbackLoads struct{ singleflight.Group }

func (s *SchedulerSnapshotService) loadCoalescedFallback(
	ctx context.Context, bucket SchedulerBucket, useMixed bool,
	token SchedulerBucketWriteToken, canPublish bool,
) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-s.stopCh:
		return nil, context.Canceled
	default:
	}
	key := bucket.String() + ":no-cache"
	if s.cache != nil {
		if !canPublish || !token.ValidFor(bucket) {
			// Unknown lifecycle state is not an epoch. Keep caller-owned fallback
			// rather than sharing a query from a potentially obsolete generation.
			fallbackCtx, cancel := s.withFallbackTimeout(ctx)
			defer cancel()
			accounts, err := s.loadFallbackSnapshot(fallbackCtx, bucket, useMixed, token, canPublish)
			if err != nil {
				return nil, err
			}
			return copySchedulableFallbackAccounts(accounts), nil
		}
		key = fmt.Sprintf("%s:epoch:%d", bucket.String(), token.Epoch)
	}
	results := s.fallbackLoads.DoChan(key, func() (value any, err error) {
		// DoChan re-panics outside the caller's handler recovery. Keep the
		// process alive without logging repository payloads or credentials.
		defer func() {
			if recover() != nil {
				slog.Error("scheduler shared database fallback panicked", "bucket", bucket.String())
				value, err = nil, errors.New("scheduler shared database fallback panicked")
			}
		}()
		timeout := schedulerSharedFallbackTimeout
		if s.cfg != nil && s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds > 0 {
			timeout = time.Duration(s.cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds) * time.Second
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		// A waiter's cancellation is local, but shutdown cancels the DB query.
		go func() {
			select {
			case <-s.stopCh:
				cancel()
			case <-loadCtx.Done():
			}
		}()
		return s.loadFallbackSnapshot(loadCtx, bucket, useMixed, token, canPublish)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-results:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if result.Err != nil {
			return nil, result.Err
		}
		accounts, ok := result.Val.([]Account)
		if !ok {
			return nil, errors.New("invalid scheduler shared fallback result")
		}
		return copySchedulableFallbackAccounts(accounts), nil
	}
}

func (s *SchedulerSnapshotService) loadFallbackSnapshot(
	ctx context.Context, bucket SchedulerBucket, useMixed bool,
	token SchedulerBucketWriteToken, canPublish bool,
) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Charge the QPS budget per actual DB query, not per waiting request.
	if err := s.guardFallback(ctx); err != nil {
		return nil, err
	}
	accounts, err := s.loadAccountsFromDB(ctx, bucket, useMixed)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.cache != nil && canPublish {
		// Publication still uses the token captured before reading the DB.
		if err := s.cache.SetSnapshot(ctx, bucket, token, accounts); err != nil {
			if errors.Is(err, ErrSchedulerBucketRetired) || errors.Is(err, ErrSchedulerBucketWriteFenced) {
				slog.Debug("[Scheduler] cache publish fenced", "bucket", bucket.String())
			} else {
				slog.Warn("[Scheduler] cache write failed", "bucket", bucket.String(), "error", err)
			}
		}
	}
	return accounts, nil
}

// Each waiter owns its slice and top-level values; nested maps/pointers keep
// the existing scheduler snapshot read-only/COW contract. The published
// snapshot keeps the configured pool; expiry is evaluated at each read.
func copySchedulableFallbackAccounts(accounts []Account) []Account {
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if accounts[i].IsSchedulable() {
			out = append(out, accounts[i])
		}
	}
	return out
}
