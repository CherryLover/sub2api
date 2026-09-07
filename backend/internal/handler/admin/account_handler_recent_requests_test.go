package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// recentRequestsRepoStub 只实现最近请求接口用到的两个方法，其余方法走内嵌的 nil 接口（调到即 panic，说明越界）。
type recentRequestsRepoStub struct {
	service.UsageLogRepository

	breakdown    *usagestats.AccountRecentRequestBreakdown
	breakdownErr error
	items        []service.UsageLog

	breakdownCalls  int
	breakdownID     int64
	breakdownStart  time.Time
	breakdownEnd    time.Time
	listCalls       int
	lastListParams  pagination.PaginationParams
	lastListFilters usagestats.UsageLogFilters
}

func (s *recentRequestsRepoStub) GetAccountRecentRequestBreakdown(_ context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountRecentRequestBreakdown, error) {
	s.breakdownCalls++
	s.breakdownID = accountID
	s.breakdownStart = startTime
	s.breakdownEnd = endTime
	if s.breakdownErr != nil {
		return nil, s.breakdownErr
	}
	if s.breakdown == nil {
		return &usagestats.AccountRecentRequestBreakdown{}, nil
	}
	return s.breakdown, nil
}

func (s *recentRequestsRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	s.listCalls++
	s.lastListParams = params
	s.lastListFilters = filters
	return s.items, &pagination.PaginationResult{Total: int64(len(s.items)), Page: 1, PageSize: params.PageSize}, nil
}

// recentRequestsConcurrencyCacheStub 只覆盖并发数与等待数两个读取，模拟 Redis 里的实时槽位。
type recentRequestsConcurrencyCacheStub struct {
	service.ConcurrencyCache

	concurrency map[int64]int
	waiting     map[int64]int
}

func (s *recentRequestsConcurrencyCacheStub) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = s.concurrency[id]
	}
	return out, nil
}

func (s *recentRequestsConcurrencyCacheStub) GetAccountWaitingCount(_ context.Context, accountID int64) (int, error) {
	return s.waiting[accountID], nil
}

func setupRecentRequestsRouter(repo *recentRequestsRepoStub, cache service.ConcurrencyCache) (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	usageSvc := service.NewAccountUsageService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	var concurrencySvc *service.ConcurrencyService
	if cache != nil {
		concurrencySvc = service.NewConcurrencyService(cache)
	}
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, usageSvc, nil, concurrencySvc, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts/:id/recent-requests", handler.GetRecentRequests)
	return router, adminSvc
}

func getRecentRequests(router *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(rec, req)
	return rec
}

type recentRequestsEnvelope struct {
	Data AccountRecentRequestsResponse `json:"data"`
}

func decodeRecentRequests(t *testing.T, rec *httptest.ResponseRecorder) AccountRecentRequestsResponse {
	t.Helper()
	var payload recentRequestsEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload), rec.Body.String())
	return payload.Data
}

func TestAccountRecentRequests_DefaultsTo15MinutesAnd50Items(t *testing.T) {
	repo := &recentRequestsRepoStub{
		breakdown: &usagestats.AccountRecentRequestBreakdown{TotalRequests: 1},
		items:     []service.UsageLog{{ID: 1, UserID: 1, APIKeyID: 10, Model: "m", CreatedAt: time.Now()}},
	}
	router, _ := setupRecentRequestsRouter(repo, nil)

	before := time.Now()
	rec := getRecentRequests(router, "/api/v1/admin/accounts/7/recent-requests")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	data := decodeRecentRequests(t, rec)
	require.Equal(t, int64(7), data.AccountID)
	require.Equal(t, 15, data.WindowMinutes)

	require.Equal(t, 1, repo.breakdownCalls)
	require.Equal(t, int64(7), repo.breakdownID)
	require.Equal(t, 15*time.Minute, repo.breakdownEnd.Sub(repo.breakdownStart))
	require.False(t, repo.breakdownEnd.Before(before), "窗口终点应是当前时间")

	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, 1, repo.lastListParams.Page)
	require.Equal(t, 50, repo.lastListParams.PageSize)
	require.Equal(t, "created_at", repo.lastListParams.SortBy)
	require.Equal(t, pagination.SortOrderDesc, repo.lastListParams.SortOrder)
	require.Equal(t, int64(7), repo.lastListFilters.AccountID)
	require.NotNil(t, repo.lastListFilters.StartTime)
	require.NotNil(t, repo.lastListFilters.EndTime)
	require.True(t, repo.lastListFilters.StartTime.Equal(repo.breakdownStart), "明细与聚合必须用同一个窗口")
	require.True(t, repo.lastListFilters.EndTime.Equal(repo.breakdownEnd), "明细与聚合必须用同一个窗口")
}

func TestAccountRecentRequests_CustomMinutesAndLimit(t *testing.T) {
	repo := &recentRequestsRepoStub{
		breakdown: &usagestats.AccountRecentRequestBreakdown{TotalRequests: 3},
		items:     []service.UsageLog{{ID: 1, CreatedAt: time.Now()}},
	}
	router, _ := setupRecentRequestsRouter(repo, nil)

	rec := getRecentRequests(router, "/api/v1/admin/accounts/7/recent-requests?minutes=60&limit=5")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	data := decodeRecentRequests(t, rec)
	require.Equal(t, 60, data.WindowMinutes)
	require.Equal(t, 60*time.Minute, repo.breakdownEnd.Sub(repo.breakdownStart))
	require.Equal(t, 5, repo.lastListParams.PageSize)
}

func TestAccountRecentRequests_BoundaryValuesAccepted(t *testing.T) {
	for _, query := range []string{"minutes=1", "minutes=1440", "limit=1", "limit=200"} {
		t.Run(query, func(t *testing.T) {
			repo := &recentRequestsRepoStub{}
			router, _ := setupRecentRequestsRouter(repo, nil)
			rec := getRecentRequests(router, "/api/v1/admin/accounts/7/recent-requests?"+query)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

func TestAccountRecentRequests_RejectsOutOfRangeParams(t *testing.T) {
	for _, query := range []string{
		"minutes=0", "minutes=1441", "minutes=-5", "minutes=abc", "minutes=1.5",
		"limit=0", "limit=201", "limit=-1", "limit=abc",
	} {
		t.Run(query, func(t *testing.T) {
			repo := &recentRequestsRepoStub{}
			router, _ := setupRecentRequestsRouter(repo, nil)
			rec := getRecentRequests(router, "/api/v1/admin/accounts/7/recent-requests?"+query)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, 0, repo.breakdownCalls, "参数非法时不应查库")
			require.Equal(t, 0, repo.listCalls, "参数非法时不应查库")
		})
	}
}

func TestAccountRecentRequests_InvalidAccountID(t *testing.T) {
	repo := &recentRequestsRepoStub{}
	router, _ := setupRecentRequestsRouter(repo, nil)
	rec := getRecentRequests(router, "/api/v1/admin/accounts/abc/recent-requests")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, repo.breakdownCalls)
}

func TestAccountRecentRequests_AccountNotFound(t *testing.T) {
	repo := &recentRequestsRepoStub{}
	router, adminSvc := setupRecentRequestsRouter(repo, nil)
	adminSvc.getAccountErr = service.ErrAccountNotFound

	rec := getRecentRequests(router, "/api/v1/admin/accounts/404/recent-requests")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, 0, repo.breakdownCalls, "账号不存在时不应查用量")
	require.Equal(t, 0, repo.listCalls)
}

func TestAccountRecentRequests_EmptyWindow(t *testing.T) {
	repo := &recentRequestsRepoStub{breakdown: &usagestats.AccountRecentRequestBreakdown{}}
	router, adminSvc := setupRecentRequestsRouter(repo, nil)
	adminSvc.getAccountResult = &service.Account{ID: 7, Name: "idle", Concurrency: 4}

	rec := getRecentRequests(router, "/api/v1/admin/accounts/7/recent-requests")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	data := decodeRecentRequests(t, rec)
	require.Equal(t, int64(0), data.TotalRequests)
	require.Equal(t, 0, data.CurrentConcurrency)
	require.Equal(t, 4, data.MaxConcurrency)
	require.Equal(t, 0, data.WaitingCount)
	require.Empty(t, data.Items)
	require.Empty(t, data.ByAPIKey)
	require.Empty(t, data.ByModel)

	// 空数组必须序列化成 []，前端直接 .length，不允许 null。
	body := rec.Body.String()
	require.Contains(t, body, `"items":[]`)
	require.Contains(t, body, `"by_api_key":[]`)
	require.Contains(t, body, `"by_model":[]`)
	require.Equal(t, 0, repo.listCalls, "整窗为空时不应再查明细")
}

func TestAccountRecentRequests_AggregatesAndItems(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	upstream := "grok-4.3"
	duration := 1200
	firstToken := 300
	repo := &recentRequestsRepoStub{
		breakdown: &usagestats.AccountRecentRequestBreakdown{
			TotalRequests: 12,
			ByAPIKey: []usagestats.AccountRecentAPIKeyStat{
				{APIKeyID: 3, Name: "jerry", Count: 10, Cost: 0.0007},
				{APIKeyID: 5, Name: "tom", Count: 2, Cost: 0.0001},
			},
			ByModel: []usagestats.AccountRecentModelStat{
				{Model: "grok-3-mini", Count: 10, Cost: 0.0007},
				{Model: "grok-4", Count: 2, Cost: 0.0001},
			},
		},
		items: []service.UsageLog{
			{
				ID:              4854,
				UserID:          1,
				APIKeyID:        3,
				AccountID:       2,
				RequestID:       "req-newest",
				Model:           "grok-4.3",
				RequestedModel:  "grok-3-mini",
				UpstreamModel:   &upstream,
				RequestType:     service.RequestTypeStream,
				DurationMs:      &duration,
				FirstTokenMs:    &firstToken,
				InputTokens:     7,
				OutputTokens:    111,
				CacheReadTokens: 5,
				TotalCost:       0.000072,
				ActualCost:      0.000036,
				CreatedAt:       now,
				User:            &service.User{ID: 1, Email: "jerry@example.com"},
				APIKey:          &service.APIKey{ID: 3, Name: "jerry", Key: "sk-live-should-not-leak"},
			},
			{
				// 用户 / Key 已删除：关联补不齐，只回 id。
				ID:          4853,
				UserID:      9,
				APIKeyID:    5,
				AccountID:   2,
				RequestID:   "req-older",
				Model:       "grok-4",
				RequestType: service.RequestTypeSync,
				CreatedAt:   now.Add(-time.Minute),
			},
		},
	}
	cache := &recentRequestsConcurrencyCacheStub{
		concurrency: map[int64]int{2: 1},
		waiting:     map[int64]int{2: 2},
	}
	router, adminSvc := setupRecentRequestsRouter(repo, cache)
	adminSvc.getAccountResult = &service.Account{
		ID:          2,
		Name:        "grok-main",
		Platform:    service.PlatformOpenAI,
		Concurrency: 3,
		Credentials: map[string]any{"api_key": "sk-top-secret-credential"},
	}

	rec := getRecentRequests(router, "/api/v1/admin/accounts/2/recent-requests?minutes=15&limit=50")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	data := decodeRecentRequests(t, rec)
	require.Equal(t, int64(2), data.AccountID)
	require.Equal(t, 15, data.WindowMinutes)
	require.Equal(t, 1, data.CurrentConcurrency)
	require.Equal(t, 3, data.MaxConcurrency)
	require.Equal(t, 2, data.WaitingCount)
	require.Equal(t, int64(12), data.TotalRequests, "total_requests 来自整窗聚合，不受 items 条数影响")

	require.Len(t, data.ByAPIKey, 2)
	require.Equal(t, int64(3), data.ByAPIKey[0].APIKeyID)
	require.Equal(t, "jerry", data.ByAPIKey[0].Name)
	require.Equal(t, int64(10), data.ByAPIKey[0].Count)
	require.InDelta(t, 0.0007, data.ByAPIKey[0].Cost, 1e-9)
	require.Len(t, data.ByModel, 2)
	require.Equal(t, "grok-3-mini", data.ByModel[0].Model)
	require.Equal(t, int64(10), data.ByModel[0].Count)

	require.Len(t, data.Items, 2)
	newest := data.Items[0]
	require.Equal(t, int64(4854), newest.ID)
	require.Equal(t, "req-newest", newest.RequestID)
	require.NotNil(t, newest.User)
	require.Equal(t, int64(1), newest.User.ID)
	require.Equal(t, "jerry@example.com", newest.User.Email)
	require.NotNil(t, newest.APIKey)
	require.Equal(t, int64(3), newest.APIKey.ID)
	require.Equal(t, "jerry", newest.APIKey.Name)
	require.Equal(t, "grok-3-mini", newest.Model, "model 应是客户端请求的模型")
	require.NotNil(t, newest.UpstreamModel)
	require.Equal(t, "grok-4.3", *newest.UpstreamModel)
	require.Equal(t, "stream", newest.RequestType)
	require.True(t, newest.Stream)
	require.NotNil(t, newest.DurationMs)
	require.Equal(t, 1200, *newest.DurationMs)
	require.NotNil(t, newest.FirstTokenMs)
	require.Equal(t, 300, *newest.FirstTokenMs)
	require.Equal(t, 7, newest.InputTokens)
	require.Equal(t, 111, newest.OutputTokens)
	require.Equal(t, 5, newest.CacheReadTokens)
	require.InDelta(t, 0.000072, newest.TotalCost, 1e-12)
	require.InDelta(t, 0.000036, newest.ActualCost, 1e-12)
	require.True(t, newest.CreatedAt.Equal(now))

	older := data.Items[1]
	require.Equal(t, int64(4853), older.ID)
	require.NotNil(t, older.User)
	require.Equal(t, int64(9), older.User.ID)
	require.Equal(t, "", older.User.Email)
	require.NotNil(t, older.APIKey)
	require.Equal(t, int64(5), older.APIKey.ID)
	require.Equal(t, "", older.APIKey.Name)
	require.Equal(t, "grok-4", older.Model, "requested_model 为空时回退 model")
	require.Nil(t, older.UpstreamModel)
	require.Equal(t, "sync", older.RequestType)
	require.False(t, older.Stream)
	require.Nil(t, older.DurationMs)

	body := rec.Body.String()
	// created_at 统一 UTC RFC3339，前端不用猜时区。
	require.Contains(t, body, `"created_at":"2026-09-07T02:30:00Z"`)
	// 响应不得带任何凭据：账号 credentials、Key 明文都不能出现。
	require.NotContains(t, body, "credentials")
	require.NotContains(t, body, "sk-top-secret-credential")
	require.NotContains(t, body, "sk-live-should-not-leak")
	require.NotContains(t, strings.ToLower(body), "proxy")
}

func TestAccountRecentRequests_RepositoryErrorIsSurfaced(t *testing.T) {
	repo := &recentRequestsRepoStub{breakdownErr: errors.New("db down")}
	router, _ := setupRecentRequestsRouter(repo, nil)

	rec := getRecentRequests(router, "/api/v1/admin/accounts/7/recent-requests")
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Equal(t, 0, repo.listCalls)
}
