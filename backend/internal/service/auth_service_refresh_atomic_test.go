//go:build unit

package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type atomicRefreshUserRepo struct {
	service.UserRepository
	user *service.User
}

func (r *atomicRefreshUserRepo) GetByID(_ context.Context, id int64) (*service.User, error) {
	if id != r.user.ID {
		return nil, service.ErrUserNotFound
	}
	return r.user, nil
}

func TestAuthRefreshTokenPairOnlyOneConcurrentWinner(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := repository.NewRefreshTokenCache(rdb)
	user := &service.User{ID: 41, Email: "race@example.com", Username: "race", Role: service.RoleUser, Status: service.StatusActive, TokenVersion: 1}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-refresh-atomic-secret", ExpireHour: 1, AccessTokenExpireMinutes: 60, RefreshTokenExpireDays: 7}}
	svc := service.NewAuthService(nil, &atomicRefreshUserRepo{user: user}, cache, cfg, nil)
	initial, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	const workers = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.RefreshTokenPair(ctx, initial.RefreshToken)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded, invalid := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, service.ErrRefreshTokenInvalid):
			invalid++
		default:
			t.Fatalf("unexpected refresh result: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, workers-1, invalid)
}

func TestAuthRefreshTokenPairFailsClosedOnRedisError(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: 0})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := repository.NewRefreshTokenCache(rdb)
	user := &service.User{ID: 42, Email: "redis-error@example.com", Username: "redis-error", Role: service.RoleUser, Status: service.StatusActive, TokenVersion: 1}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-refresh-error-secret", ExpireHour: 1, AccessTokenExpireMinutes: 60, RefreshTokenExpireDays: 7}}
	svc := service.NewAuthService(nil, &atomicRefreshUserRepo{user: user}, cache, cfg, nil)
	initial, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	mr.SetError("ERR simulated redis failure")
	_, err = svc.RefreshTokenPair(ctx, initial.RefreshToken)
	require.ErrorIs(t, err, service.ErrServiceUnavailable)
}
