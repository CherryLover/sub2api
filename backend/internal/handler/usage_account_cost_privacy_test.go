package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserUsageEndpointStatsNeverExposeAccountCost(t *testing.T) {
	cost := 12.0
	repo := &userUsageRepoCapture{stats: &usagestats.UsageStats{TotalAccountCost: &cost, Endpoints: []usagestats.EndpointStat{{Endpoint: "/v1/messages", Cost: 4, ActualCost: 2, AccountCost: 12}}, UpstreamEndpoints: []usagestats.EndpointStat{{Endpoint: "private-route", AccountCost: 12}}}}
	rec := httptest.NewRecorder()
	newUserUsageRequestTypeTestRouter(repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/usage/stats", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "account_cost")
	require.NotContains(t, rec.Body.String(), "private-route")
	require.Equal(t, float64(2), gjson.GetBytes(rec.Body.Bytes(), "data.endpoints.0.actual_cost").Float())
}
