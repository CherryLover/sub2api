//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamErrorRequestsHandlerRepo struct {
	service.OpsRepository
	listFilter    *service.OpsUpstreamErrorRequestFilter
	summaryFilter *service.OpsUpstreamErrorRequestFilter
	trendFilter   *service.OpsUpstreamErrorRequestFilter
	listCalls     int
	summaryCalls  int
	trendCalls    int
	err           error
}

func (r *upstreamErrorRequestsHandlerRepo) ListUpstreamErrorRequests(_ context.Context, filter *service.OpsUpstreamErrorRequestFilter) (*service.OpsUpstreamErrorRequestList, error) {
	r.listCalls++
	snapshot := *filter
	r.listFilter = &snapshot
	return &service.OpsUpstreamErrorRequestList{
		Items:    []*service.OpsUpstreamErrorRequest{},
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, r.err
}

func (r *upstreamErrorRequestsHandlerRepo) SummarizeUpstreamErrorRequests(_ context.Context, filter *service.OpsUpstreamErrorRequestFilter) (*service.OpsUpstreamErrorRequestSummary, error) {
	r.summaryCalls++
	snapshot := *filter
	r.summaryFilter = &snapshot
	return &service.OpsUpstreamErrorRequestSummary{
		Total:          12,
		Recovered:      7,
		Failed:         5,
		Attempts:       19,
		FailedByStatus: []service.OpsUpstreamErrorStatusCount{{StatusCode: 503, Count: 3}},
	}, r.err
}

func (r *upstreamErrorRequestsHandlerRepo) GetUpstreamErrorRequestTrend(_ context.Context, filter *service.OpsUpstreamErrorRequestFilter) ([]*service.OpsUpstreamErrorTrendDay, error) {
	r.trendCalls++
	snapshot := *filter
	r.trendFilter = &snapshot
	return []*service.OpsUpstreamErrorTrendDay{}, r.err
}

func newUpstreamErrorRequestsRouter(t *testing.T, repo service.OpsRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewOpsHandler(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	router := gin.New()
	router.GET("/upstream-error-requests", h.ListUpstreamErrorRequests)
	router.GET("/upstream-error-requests/summary", h.GetUpstreamErrorRequestsSummary)
	router.GET("/upstream-error-requests/trend", h.GetUpstreamErrorRequestsTrend)
	return router
}

func upstreamErrorRequestsGET(ops *service.OpsService, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/upstream-error-requests", NewOpsHandler(ops).ListUpstreamErrorRequests)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/upstream-error-requests?"+query, nil))
	return recorder
}

func TestOpsUpstreamErrorRequestsHandler_InvalidParameters(t *testing.T) {
	for _, param := range []string{"page", "page_size", "status_code", "api_key_id", "account_id"} {
		for _, value := range []string{"abc", "0", "-1", "92233720368547758080"} {
			t.Run(param+"/"+value, func(t *testing.T) {
				repo := &upstreamErrorRequestsHandlerRepo{}
				ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				recorder := upstreamErrorRequestsGET(ops, url.Values{param: {value}}.Encode())
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), "Invalid "+param)
				require.Zero(t, repo.listCalls)
			})
		}
	}
	for _, query := range []string{
		"start_time=bad",
		"end_time=2026-11-04",
		"status_code=99",
		"status_code=600",
		"final_status_code=99",
		"final_status_code=600",
		"outcome=unknown",
		"page=10000001",
		"start_time=2026-11-04T02:00:00Z&end_time=2026-11-04T01:00:00Z",
	} {
		t.Run(query, func(t *testing.T) {
			repo := &upstreamErrorRequestsHandlerRepo{}
			recorder := upstreamErrorRequestsGET(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), query)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.Zero(t, repo.listCalls)
		})
	}
}

func TestOpsUpstreamErrorRequestsHandler_FiltersAndShape(t *testing.T) {
	repo := &upstreamErrorRequestsHandlerRepo{}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	recorder := upstreamErrorRequestsGET(ops, "page=2&page_size=101&status_code=503&final_status_code=429&api_key_id=7&account_id=9&outcome=recovered&start_time=2026-11-04T01:00:00Z&end_time=2026-11-04T02:00:00Z")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, &service.OpsUpstreamErrorRequestFilter{
		Page: 2, PageSize: 100, StatusCode: 503, FinalStatusCode: 429,
		APIKeyID: 7, AccountID: 9, Outcome: "recovered",
		StartTime: time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC),
	}, repo.listFilter)
	var body struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.Equal(t, []any{}, body.Data["items"])
	require.EqualValues(t, 2, body.Data["page"])
	require.EqualValues(t, 100, body.Data["page_size"])
	require.EqualValues(t, 0, body.Data["total"])
}

func TestOpsUpstreamErrorRequestsHandler_UnavailableAndRepositoryError(t *testing.T) {
	require.Equal(t, http.StatusServiceUnavailable, upstreamErrorRequestsGET(nil, "").Code)
	repo := &upstreamErrorRequestsHandlerRepo{err: errors.New("database failed")}
	recorder := upstreamErrorRequestsGET(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), "")
	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.listCalls)
}

func TestUpstreamErrorRequestsHandlers_ForwardListAndSummaryFilters(t *testing.T) {
	for _, path := range []string{"/upstream-error-requests", "/upstream-error-requests/summary"} {
		t.Run(path, func(t *testing.T) {
			repo := &upstreamErrorRequestsHandlerRepo{}
			router := newUpstreamErrorRequestsRouter(t, repo)
			query := url.Values{
				"page":              {"3"},
				"page_size":         {"50"},
				"status_code":       {"418"},
				"final_status_code": {"503"},
				"api_key_id":        {"7"},
				"account_id":        {"9"},
				"outcome":           {"failed"},
				"start_time":        {"2026-11-04T01:00:00Z"},
				"end_time":          {"2026-11-04T02:00:00Z"},
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path+"?"+query.Encode(), nil))

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var got *service.OpsUpstreamErrorRequestFilter
			if path == "/upstream-error-requests" {
				require.Equal(t, 1, repo.listCalls)
				require.Zero(t, repo.summaryCalls)
				got = repo.listFilter
			} else {
				require.Zero(t, repo.listCalls)
				require.Equal(t, 1, repo.summaryCalls)
				got = repo.summaryFilter
			}
			require.NotNil(t, got)
			require.Equal(t, 3, got.Page)
			require.Equal(t, 50, got.PageSize)
			require.Equal(t, 418, got.StatusCode)
			require.Equal(t, 503, got.FinalStatusCode)
			require.Equal(t, int64(7), got.APIKeyID)
			require.Equal(t, int64(9), got.AccountID)
			require.Equal(t, "failed", got.Outcome)
			require.Equal(t, time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC), got.StartTime)
			require.Equal(t, time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC), got.EndTime)
		})
	}
}

func TestUpstreamErrorRequestsTrendHandler_ForwardsDaysAndOverridesTime(t *testing.T) {
	repo := &upstreamErrorRequestsHandlerRepo{}
	router := newUpstreamErrorRequestsRouter(t, repo)
	query := url.Values{
		"days":              {"14"},
		"status_code":       {"429"},
		"final_status_code": {"503"},
		"api_key_id":        {"7"},
		"account_id":        {"9"},
		"outcome":           {"failed"},
		"start_time":        {"2020-01-01T00:00:00Z"},
		"end_time":          {"2020-01-02T00:00:00Z"},
	}
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/upstream-error-requests/trend?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.trendCalls)
	require.NotNil(t, repo.trendFilter)
	require.Equal(t, 429, repo.trendFilter.StatusCode)
	require.Equal(t, 503, repo.trendFilter.FinalStatusCode)
	require.Equal(t, int64(7), repo.trendFilter.APIKeyID)
	require.Equal(t, int64(9), repo.trendFilter.AccountID)
	require.Equal(t, "failed", repo.trendFilter.Outcome)
	require.Equal(t, 14*24*time.Hour, repo.trendFilter.EndTime.Sub(repo.trendFilter.StartTime))
	require.NotEqual(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), repo.trendFilter.StartTime)
}

func TestUpstreamErrorRequestsHandlers_InvalidFiltersDoNotQuery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  string
		query string
	}{
		{name: "list status low", path: "/upstream-error-requests", query: "status_code=99"},
		{name: "list status high", path: "/upstream-error-requests", query: "status_code=600"},
		{name: "summary final status low", path: "/upstream-error-requests/summary", query: "final_status_code=99"},
		{name: "summary final status high", path: "/upstream-error-requests/summary", query: "final_status_code=600"},
		{name: "trend days zero", path: "/upstream-error-requests/trend", query: "days=0"},
		{name: "trend days unsupported", path: "/upstream-error-requests/trend", query: "days=8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamErrorRequestsHandlerRepo{}
			router := newUpstreamErrorRequestsRouter(t, repo)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path+"?"+tc.query, nil))

			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.Zero(t, repo.listCalls)
			require.Zero(t, repo.summaryCalls)
			require.Zero(t, repo.trendCalls)
		})
	}
}
