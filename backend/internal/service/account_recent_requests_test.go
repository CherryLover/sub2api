package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// accountRecentRequestsRepoStub 同时实现整窗聚合与明细分页两个入口。
type accountRecentRequestsRepoStub struct {
	UsageLogRepository

	breakdown    *usagestats.AccountRecentRequestBreakdown
	breakdownErr error
	items        []UsageLog
	listErr      error

	listCalls   int
	lastParams  pagination.PaginationParams
	lastFilters usagestats.UsageLogFilters
}

func (s *accountRecentRequestsRepoStub) GetAccountRecentRequestBreakdown(_ context.Context, _ int64, _, _ time.Time) (*usagestats.AccountRecentRequestBreakdown, error) {
	if s.breakdownErr != nil {
		return nil, s.breakdownErr
	}
	return s.breakdown, nil
}

func (s *accountRecentRequestsRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error) {
	s.listCalls++
	s.lastParams = params
	s.lastFilters = filters
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	return s.items, &pagination.PaginationResult{Total: int64(len(s.items))}, nil
}

// accountRecentRequestsPlainRepoStub 故意不实现整窗聚合，模拟旧仓储。
type accountRecentRequestsPlainRepoStub struct {
	UsageLogRepository
}

func newAccountRecentRequestsService(repo UsageLogRepository) *AccountUsageService {
	return NewAccountUsageService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func TestGetAccountRecentRequests_RepositoryWithoutBreakdownIsRejected(t *testing.T) {
	svc := newAccountRecentRequestsService(&accountRecentRequestsPlainRepoStub{})
	now := time.Now()
	_, err := svc.GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 10)
	require.ErrorIs(t, err, ErrAccountRecentRequestsUnsupported)
}

func TestGetAccountRecentRequests_NilServiceOrRepoIsRejected(t *testing.T) {
	now := time.Now()
	var nilSvc *AccountUsageService
	_, err := nilSvc.GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 10)
	require.ErrorIs(t, err, ErrAccountRecentRequestsUnsupported)

	_, err = newAccountRecentRequestsService(nil).GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 10)
	require.ErrorIs(t, err, ErrAccountRecentRequestsUnsupported)
}

func TestGetAccountRecentRequests_EmptyWindowSkipsDetailQuery(t *testing.T) {
	repo := &accountRecentRequestsRepoStub{breakdown: &usagestats.AccountRecentRequestBreakdown{}}
	svc := newAccountRecentRequestsService(repo)
	now := time.Now()

	result, err := svc.GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 10)
	require.NoError(t, err)
	require.Equal(t, int64(0), result.TotalRequests)
	require.NotNil(t, result.Items)
	require.Empty(t, result.Items)
	require.NotNil(t, result.ByAPIKey)
	require.Empty(t, result.ByAPIKey)
	require.NotNil(t, result.ByModel)
	require.Empty(t, result.ByModel)
	require.Equal(t, 0, repo.listCalls, "整窗为空时不应再查明细")
}

func TestGetAccountRecentRequests_QueriesNewestItemsWithSameWindow(t *testing.T) {
	start := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	end := start.Add(15 * time.Minute)
	repo := &accountRecentRequestsRepoStub{
		breakdown: &usagestats.AccountRecentRequestBreakdown{
			TotalRequests: 5,
			ByAPIKey:      []usagestats.AccountRecentAPIKeyStat{{APIKeyID: 3, Name: "k", Count: 5, Cost: 1}},
			ByModel:       []usagestats.AccountRecentModelStat{{Model: "m", Count: 5, Cost: 1}},
		},
		items: []UsageLog{{ID: 2}, {ID: 1}},
	}
	svc := newAccountRecentRequestsService(repo)

	result, err := svc.GetAccountRecentRequests(context.Background(), 42, start, end, 2)
	require.NoError(t, err)
	require.Equal(t, int64(5), result.TotalRequests)
	require.Len(t, result.Items, 2)
	require.Equal(t, int64(2), result.Items[0].ID)
	require.Len(t, result.ByAPIKey, 1)
	require.Len(t, result.ByModel, 1)
	require.True(t, result.StartTime.Equal(start))
	require.True(t, result.EndTime.Equal(end))

	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, 1, repo.lastParams.Page)
	require.Equal(t, 2, repo.lastParams.PageSize)
	require.Equal(t, "created_at", repo.lastParams.SortBy)
	require.Equal(t, pagination.SortOrderDesc, repo.lastParams.SortOrder)
	require.Equal(t, int64(42), repo.lastFilters.AccountID)
	require.NotNil(t, repo.lastFilters.StartTime)
	require.NotNil(t, repo.lastFilters.EndTime)
	require.True(t, repo.lastFilters.StartTime.Equal(start))
	require.True(t, repo.lastFilters.EndTime.Equal(end))
	require.False(t, repo.lastFilters.ExactTotal, "账号过滤本身就走精确计数，不必额外要求")
}

func TestGetAccountRecentRequests_NonPositiveLimitOnlyAggregates(t *testing.T) {
	repo := &accountRecentRequestsRepoStub{
		breakdown: &usagestats.AccountRecentRequestBreakdown{TotalRequests: 3},
		items:     []UsageLog{{ID: 1}},
	}
	svc := newAccountRecentRequestsService(repo)
	now := time.Now()

	result, err := svc.GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 0)
	require.NoError(t, err)
	require.Equal(t, int64(3), result.TotalRequests)
	require.Empty(t, result.Items)
	require.Equal(t, 0, repo.listCalls)
}

func TestGetAccountRecentRequests_ErrorsAreWrapped(t *testing.T) {
	now := time.Now()

	breakdownErr := errors.New("breakdown boom")
	svc := newAccountRecentRequestsService(&accountRecentRequestsRepoStub{breakdownErr: breakdownErr})
	_, err := svc.GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 10)
	require.ErrorIs(t, err, breakdownErr)

	listErr := errors.New("list boom")
	svc = newAccountRecentRequestsService(&accountRecentRequestsRepoStub{
		breakdown: &usagestats.AccountRecentRequestBreakdown{TotalRequests: 1},
		listErr:   listErr,
	})
	_, err = svc.GetAccountRecentRequests(context.Background(), 1, now.Add(-time.Minute), now, 10)
	require.ErrorIs(t, err, listErr)
}
