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
	filter *service.OpsUpstreamErrorRequestFilter
	calls  int
	err    error
}

func (r *upstreamErrorRequestsHandlerRepo) ListUpstreamErrorRequests(_ context.Context, f *service.OpsUpstreamErrorRequestFilter) (*service.OpsUpstreamErrorRequestList, error) {
	r.calls++
	copy := *f
	r.filter = &copy
	return &service.OpsUpstreamErrorRequestList{Items: []*service.OpsUpstreamErrorRequest{}, Page: f.Page, PageSize: f.PageSize}, r.err
}
func upstreamErrorRequestsGET(ops *service.OpsService, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/upstream-errors", NewOpsHandler(ops).ListUpstreamErrorRequests)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/upstream-errors?"+query, nil))
	return rec
}
func TestOpsUpstreamErrorRequestsHandler_InvalidParameters(t *testing.T) {
	for _, param := range []string{"page", "page_size", "status_code", "api_key_id", "account_id"} {
		for _, value := range []string{"abc", "0", "-1", "92233720368547758080"} {
			t.Run(param+"/"+value, func(t *testing.T) {
				repo := &upstreamErrorRequestsHandlerRepo{}
				ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				rec := upstreamErrorRequestsGET(ops, url.Values{param: {value}}.Encode())
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "Invalid "+param)
				require.Zero(t, repo.calls)
			})
		}
	}
	for _, query := range []string{"start_time=bad", "end_time=2026-11-04", "status_code=500", "outcome=unknown", "page=10000001", "start_time=2026-11-04T02:00:00Z&end_time=2026-11-04T01:00:00Z"} {
		t.Run(query, func(t *testing.T) {
			repo := &upstreamErrorRequestsHandlerRepo{}
			rec := upstreamErrorRequestsGET(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), query)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Zero(t, repo.calls)
		})
	}
}
func TestOpsUpstreamErrorRequestsHandler_FiltersAndShape(t *testing.T) {
	repo := &upstreamErrorRequestsHandlerRepo{}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	rec := upstreamErrorRequestsGET(ops, "page=2&page_size=101&status_code=503&api_key_id=7&account_id=9&outcome=recovered&start_time=2026-11-04T01:00:00Z&end_time=2026-11-04T02:00:00Z")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, repo.calls)
	require.Equal(t, &service.OpsUpstreamErrorRequestFilter{Page: 2, PageSize: 100, StatusCode: 503, APIKeyID: 7, AccountID: 9, Outcome: "recovered", StartTime: time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC), EndTime: time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC)}, repo.filter)
	var body struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.Equal(t, []any{}, body.Data["items"])
	require.EqualValues(t, 2, body.Data["page"])
	require.EqualValues(t, 100, body.Data["page_size"])
	require.EqualValues(t, 0, body.Data["total"])
}
func TestOpsUpstreamErrorRequestsHandler_UnavailableAndRepositoryError(t *testing.T) {
	require.Equal(t, http.StatusServiceUnavailable, upstreamErrorRequestsGET(nil, "").Code)
	repo := &upstreamErrorRequestsHandlerRepo{err: errors.New("database failed")}
	rec := upstreamErrorRequestsGET(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), "")
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	require.Equal(t, 1, repo.calls)
}
