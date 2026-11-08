package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type keyUsageDashboardOwnershipRepoStub struct {
	service.APIKeyRepository
	keys map[int64]*service.APIKey
}

func (s *keyUsageDashboardOwnershipRepoStub) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	key, ok := s.keys[id]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *key
	return &clone, nil
}

func newKeyUsageDashboardHandlerTestRouter(userID int64, repo *keyUsageDashboardOwnershipRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil)
	usageHandler := NewUsageHandler(nil, apiKeyService, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
		c.Next()
	})
	router.GET("/usage/key-dashboard", usageHandler.KeyUsageDashboard)
	router.GET("/usage/key-dashboard/keys/:id", usageHandler.KeyUsageDashboardKeyDetail)
	return router
}

func TestParseKeyUsageDashboardQuery(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		want       *keyUsageDashboardQuery
	}{
		{name: "missing range", query: "", wantStatus: http.StatusBadRequest},
		{name: "invalid range", query: "?range=90d", wantStatus: http.StatusBadRequest},
		{name: "invalid group", query: "?range=7d&group_id=x", wantStatus: http.StatusBadRequest},
		{name: "zero group", query: "?range=7d&group_id=0", wantStatus: http.StatusBadRequest},
		{
			name:       "valid",
			query:      "?range=30d&group_id=12&model=%20gpt-5%20",
			wantStatus: http.StatusOK,
			want: &keyUsageDashboardQuery{
				Range:   "30d",
				GroupID: 12,
				Model:   "gpt-5",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			request := httptest.NewRequest(http.MethodGet, "/usage/key-dashboard"+test.query, nil)
			context.Request = request

			got, ok := parseKeyUsageDashboardQuery(context)
			if test.wantStatus == http.StatusBadRequest {
				require.False(t, ok)
				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Nil(t, got)
				return
			}
			require.True(t, ok)
			require.Equal(t, test.want, got)
		})
	}
}

func TestKeyUsageDashboardHandlersValidateRangeBeforeService(t *testing.T) {
	repo := &keyUsageDashboardOwnershipRepoStub{
		keys: map[int64]*service.APIKey{
			7: {ID: 7, UserID: 42, Name: "owned", Status: service.StatusAPIKeyActive},
		},
	}
	router := newKeyUsageDashboardHandlerTestRouter(42, repo)

	for _, path := range []string{
		"/usage/key-dashboard",
		"/usage/key-dashboard?range=90d",
		"/usage/key-dashboard/keys/7",
		"/usage/key-dashboard/keys/7?range=90d",
		"/usage/key-dashboard/keys/not-a-number?range=7d",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestKeyUsageDashboardKeyDetailReturnsNotFoundForCrossUserKey(t *testing.T) {
	repo := &keyUsageDashboardOwnershipRepoStub{
		keys: map[int64]*service.APIKey{
			7: {ID: 7, UserID: 99, Name: "other", Status: service.StatusAPIKeyActive},
		},
	}
	router := newKeyUsageDashboardHandlerTestRouter(42, repo)
	request := httptest.NewRequest(http.MethodGet, "/usage/key-dashboard/keys/7?range=7d", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestKeyUsageDashboardKeyDetailReturnsNotFoundForMissingKey(t *testing.T) {
	repo := &keyUsageDashboardOwnershipRepoStub{keys: map[int64]*service.APIKey{}}
	router := newKeyUsageDashboardHandlerTestRouter(42, repo)
	request := httptest.NewRequest(http.MethodGet, "/usage/key-dashboard/keys/404?range=7d", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}
