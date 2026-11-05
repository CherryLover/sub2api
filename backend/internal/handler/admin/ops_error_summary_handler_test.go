//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type errorSummaryHandlerRepo struct {
	service.OpsRepository
	listFilter    *service.OpsErrorLogFilter
	summaryFilter *service.OpsErrorLogFilter
	listCalls     int
	summaryCalls  int
}

func (r *errorSummaryHandlerRepo) ListErrorLogs(_ context.Context, filter *service.OpsErrorLogFilter) (*service.OpsErrorLogList, error) {
	r.listCalls++
	snapshot := *filter
	r.listFilter = &snapshot
	return &service.OpsErrorLogList{Errors: []*service.OpsErrorLog{}, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *errorSummaryHandlerRepo) SummarizeErrorLogs(_ context.Context, filter *service.OpsErrorLogFilter) (*service.OpsErrorLogSummary, error) {
	r.summaryCalls++
	snapshot := *filter
	r.summaryFilter = &snapshot
	return &service.OpsErrorLogSummary{
		Total: 7,
		Items: []*service.OpsErrorKindSummary{{
			Kind: "routing|503|model_rate_limited", Phase: "routing", StatusCode: 503,
			Reason: "model_rate_limited", SampleMessage: "Service temporarily unavailable",
			Count: 7, Failed: 7,
		}},
	}, nil
}

func newErrorSummaryRouter(t *testing.T, repo service.OpsRepository) (*gin.Engine, *OpsHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewOpsHandler(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	router := gin.New()
	router.GET("/errors", h.GetErrorLogs)
	router.GET("/errors/summary", h.GetErrorLogsSummary)
	return router, h
}

func TestOpsErrorSummaryHandler_IgnoresKindPaginationAndSort(t *testing.T) {
	repo := &errorSummaryHandlerRepo{}
	router, _ := newErrorSummaryRouter(t, repo)
	query := url.Values{
		"view":           {"all"},
		"error_kind":     {"routing|503|model_rate_limited"},
		"page":           {"3"},
		"page_size":      {"50"},
		"sort_by":        {"model"},
		"sort_order":     {"asc"},
		"group_id":       {"3"},
		"account_id":     {"9"},
		"user_id":        {"42"},
		"api_key_id":     {"7"},
		"model":          {"gpt-5.5"},
		"phase":          {"routing"},
		"category":       {"rate_limit"},
		"status_codes":   {"503,529"},
		"error_owner":    {"platform"},
		"error_source":   {"gateway"},
		"start_time":     {"2026-11-04T01:00:00Z"},
		"end_time":       {"2026-11-04T02:00:00Z"},
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/errors/summary?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.summaryCalls)
	require.Zero(t, repo.listCalls)
	require.NotNil(t, repo.summaryFilter)
	require.Empty(t, repo.summaryFilter.ErrorKind)
	require.Zero(t, repo.summaryFilter.Page)
	require.Zero(t, repo.summaryFilter.PageSize)
	require.Empty(t, repo.summaryFilter.SortBy)
	require.Empty(t, repo.summaryFilter.SortOrder)
	require.False(t, repo.summaryFilter.IncludeRecoveredUpstream)
	require.Equal(t, "all", repo.summaryFilter.View)
	require.Equal(t, "gpt-5.5", repo.summaryFilter.Model)
	require.Equal(t, "routing", repo.summaryFilter.Phase)
	require.Equal(t, []int{503, 529}, repo.summaryFilter.StatusCodes)
	require.Equal(t, "platform", repo.summaryFilter.Owner)
	require.Equal(t, "gateway", repo.summaryFilter.Source)
}

func TestOpsErrorListHandler_ForwardsErrorKind(t *testing.T) {
	repo := &errorSummaryHandlerRepo{}
	router, _ := newErrorSummaryRouter(t, repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet,
		"/errors?error_kind="+url.QueryEscape("routing|503|model_rate_limited"),
		nil,
	))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, repo.listCalls)
	require.Zero(t, repo.summaryCalls)
	require.NotNil(t, repo.listFilter)
	require.Equal(t, "routing|503|model_rate_limited", repo.listFilter.ErrorKind)
}

func TestOpsErrorHandlers_ForwardMinRequestBodyBytes(t *testing.T) {
	repo := &errorSummaryHandlerRepo{}
	router, _ := newErrorSummaryRouter(t, repo)
	for _, path := range []string{"/errors", "/errors/summary"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path+"?min_request_body_bytes=4194304", nil))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}
	require.EqualValues(t, 4194304, repo.listFilter.MinRequestBodyBytes)
	require.EqualValues(t, 4194304, repo.summaryFilter.MinRequestBodyBytes)
}

func TestOpsErrorListHandler_RejectsSyntheticOther(t *testing.T) {
	repo := &errorSummaryHandlerRepo{}
	router, _ := newErrorSummaryRouter(t, repo)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/errors?error_kind=other", nil))

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Zero(t, repo.listCalls)
}

func TestOpsErrorSummaryHandler_InvalidFilters(t *testing.T) {
	for _, query := range []string{
		"start_time=bad",
		"group_id=0",
		"account_id=-1",
		"user_id=abc",
		"api_key_id=0",
		"resolved=maybe",
		"status_codes=503,bad",
		"min_request_body_bytes=-1",
		"min_request_body_bytes=2MB",
	} {
		t.Run(query, func(t *testing.T) {
			repo := &errorSummaryHandlerRepo{}
			router, _ := newErrorSummaryRouter(t, repo)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/errors/summary?"+query, nil))
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.Zero(t, repo.summaryCalls)
		})
	}
}
