package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type breakdownRepoCapture struct {
	service.UsageLogRepository

	capturedDim    usagestats.UserBreakdownDimension
	capturedLimit  int
	capturedDimKey usagestats.BreakdownModelDimension
	capturedIDs    []int64

	users   []usagestats.UserBreakdownItem
	apiKeys []usagestats.APIKeyBreakdownItem
	shares  map[int64][]usagestats.BreakdownModelShare
}

func (r *breakdownRepoCapture) GetUserBreakdownStats(
	_ context.Context, _, _ time.Time,
	dim usagestats.UserBreakdownDimension, limit int,
) ([]usagestats.UserBreakdownItem, error) {
	r.capturedDim = dim
	r.capturedLimit = limit
	return r.users, nil
}

func (r *breakdownRepoCapture) GetAPIKeyBreakdownStats(
	_ context.Context, _, _ time.Time,
	dim usagestats.UserBreakdownDimension, limit int,
) ([]usagestats.APIKeyBreakdownItem, error) {
	r.capturedDim = dim
	r.capturedLimit = limit
	return r.apiKeys, nil
}

func (r *breakdownRepoCapture) GetBreakdownModelShares(
	_ context.Context, _, _ time.Time,
	_ usagestats.UserBreakdownDimension, modelDim usagestats.BreakdownModelDimension, ids []int64,
) (map[int64][]usagestats.BreakdownModelShare, error) {
	r.capturedDimKey = modelDim
	r.capturedIDs = ids
	if r.shares != nil {
		return r.shares, nil
	}
	return map[int64][]usagestats.BreakdownModelShare{}, nil
}

func newBreakdownRouter(repo *breakdownRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewDashboardService(repo, nil, nil, nil)
	h := NewDashboardHandler(svc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/user-breakdown", h.GetUserBreakdown)
	router.GET("/admin/dashboard/api-key-breakdown", h.GetAPIKeyBreakdown)
	return router
}

func TestGetAPIKeyBreakdown_PassesFiltersAndLimit(t *testing.T) {
	repo := &breakdownRepoCapture{
		apiKeys: []usagestats.APIKeyBreakdownItem{{APIKeyID: 7, Name: "king jin", ActualCost: 537.78}},
	}
	router := newBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/api-key-breakdown?start_date=2026-11-09&end_date=2026-11-09&user_id=2&limit=20&sort_by=requests", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(2), repo.capturedDim.UserID)
	require.Equal(t, "requests", repo.capturedDim.SortBy)
	require.Equal(t, 20, repo.capturedLimit)

	var payload struct {
		Data struct {
			APIKeys []usagestats.APIKeyBreakdownItem `json:"api_keys"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Data.APIKeys, 1)
	require.Equal(t, "king jin", payload.Data.APIKeys[0].Name)
}

func TestGetAPIKeyBreakdown_IncludeModelsAttachesShares(t *testing.T) {
	repo := &breakdownRepoCapture{
		apiKeys: []usagestats.APIKeyBreakdownItem{
			{APIKeyID: 7, Name: "king jin"},
			{APIKeyID: 8, Name: "Fengshijie"},
		},
		shares: map[int64][]usagestats.BreakdownModelShare{
			7: {{Model: "gpt-6-astra", Requests: 1820, ActualCost: 395.15}},
		},
	}
	router := newBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/api-key-breakdown?start_date=2026-11-09&end_date=2026-11-09&include_models=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, usagestats.BreakdownModelByAPIKey, repo.capturedDimKey)
	require.Equal(t, []int64{7, 8}, repo.capturedIDs)

	var payload struct {
		Data struct {
			APIKeys []usagestats.APIKeyBreakdownItem `json:"api_keys"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Data.APIKeys[0].Models, 1)
	require.Equal(t, "gpt-6-astra", payload.Data.APIKeys[0].Models[0].Model)
	require.Empty(t, payload.Data.APIKeys[1].Models)
}

func TestGetAPIKeyBreakdown_WithoutIncludeModelsSkipsDetailQuery(t *testing.T) {
	repo := &breakdownRepoCapture{
		apiKeys: []usagestats.APIKeyBreakdownItem{{APIKeyID: 7}},
	}
	router := newBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/api-key-breakdown?start_date=2026-11-09&end_date=2026-11-09", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, repo.capturedDimKey)
	require.Empty(t, repo.capturedIDs)
}

func TestGetAPIKeyBreakdown_RejectsInvalidModelSource(t *testing.T) {
	repo := &breakdownRepoCapture{}
	router := newBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/api-key-breakdown?start_date=2026-11-09&end_date=2026-11-09&model_source=foobar", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetUserBreakdown_IncludeModelsAttachesShares(t *testing.T) {
	repo := &breakdownRepoCapture{
		users: []usagestats.UserBreakdownItem{{UserID: 2, Email: "it@xthings.com"}},
		shares: map[int64][]usagestats.BreakdownModelShare{
			2: {{Model: "gpt-6-astra", Requests: 2236, ActualCost: 581.92}},
		},
	}
	router := newBreakdownRouter(repo)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/dashboard/user-breakdown?start_date=2026-11-09&end_date=2026-11-09&include_models=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, usagestats.BreakdownModelByUser, repo.capturedDimKey)
	require.Equal(t, []int64{2}, repo.capturedIDs)

	var payload struct {
		Data struct {
			Users []usagestats.UserBreakdownItem `json:"users"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Users[0].Models, 1)
}
