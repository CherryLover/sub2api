package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// --- 测试替身 -------------------------------------------------------------

type stubKeyUsageAPIKeys struct {
	byKey map[string]*service.APIKey
	byID  map[int64]*service.APIKey
}

func (s *stubKeyUsageAPIKeys) GetByKey(_ context.Context, key string) (*service.APIKey, error) {
	if apiKey, ok := s.byKey[key]; ok {
		return apiKey, nil
	}
	return nil, service.ErrAPIKeyNotFound
}

func (s *stubKeyUsageAPIKeys) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	if apiKey, ok := s.byID[id]; ok {
		return apiKey, nil
	}
	return nil, service.ErrAPIKeyNotFound
}

type stubKeyUsageModelStats struct{}

func (stubKeyUsageModelStats) GetAPIKeyModelStats(_ context.Context, apiKeyID int64, _, _ time.Time) ([]usagestats.ModelStat, error) {
	return []usagestats.ModelStat{{Model: "claude-opus-5", Requests: 3, TotalTokens: 900, ActualCost: 1.5}}, nil
}

type stubKeyUsageRanking struct{}

func (stubKeyUsageRanking) GetAPIKeyUsageAggregates(_ context.Context, _, _ time.Time, _ int64, _ string) ([]usagestats.APIKeyUsageAggregate, error) {
	return []usagestats.APIKeyUsageAggregate{
		{APIKeyID: 2, Requests: 10, Tokens: 4000, Cost: 9.9},
		{APIKeyID: 1, Requests: 3, Tokens: 900, Cost: 1.5},
	}, nil
}

func (stubKeyUsageRanking) GetAPIKeyNamesByIDs(_ context.Context, ids []int64) (map[int64]string, error) {
	names := map[int64]string{1: "我的Key", 2: "别人的Key"}
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		out[id] = names[id]
	}
	return out, nil
}

func newTestKeyUsageRouter(t *testing.T) (*gin.Engine, *service.APIKey, *service.KeyUsageService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	apiKey := &service.APIKey{ID: 1, UserID: 7, Key: "sk-live-key", Name: "我的Key", Status: service.StatusAPIKeyActive, CreatedAt: time.Now()}
	disabled := &service.APIKey{ID: 2, UserID: 7, Key: "sk-disabled-key", Name: "禁用Key", Status: service.StatusAPIKeyDisabled, CreatedAt: time.Now()}
	keys := &stubKeyUsageAPIKeys{
		byKey: map[string]*service.APIKey{apiKey.Key: apiKey, disabled.Key: disabled},
		byID:  map[int64]*service.APIKey{apiKey.ID: apiKey, disabled.ID: disabled},
	}

	tokens := service.NewKeyUsageTokenService(service.DeriveKeyUsageTokenSigningKey("handler-test-master-secret-0123456789"), time.Hour)
	svc := service.NewKeyUsageService(keys, stubKeyUsageModelStats{}, stubKeyUsageRanking{}, tokens, time.Minute)

	// gateway 传 nil：本用例只验证公开端点的契约形状，usage 字段退化成空对象。
	h := NewKeyUsageHandler(svc, nil, nil)
	router := gin.New()
	group := router.Group("/api/v1/key-usage")
	group.POST("/session", h.CreateSession)
	group.GET("/report", h.Report)
	return router, apiKey, svc
}

func doKeyUsageRequest(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// --- session ---------------------------------------------------------------

func TestKeyUsageCreateSessionReturnsToken(t *testing.T) {
	router, apiKey, _ := newTestKeyUsageRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/key-usage/session", strings.NewReader(`{"key":"`+apiKey.Key+`"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := doKeyUsageRequest(router, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.NotEmpty(t, body.Token)
	require.NotContains(t, body.Token, apiKey.Key, "令牌不能包含原始 key")
	require.True(t, body.ExpiresAt.After(time.Now()))
}

// 401 路径不能泄露 key 是否存在：无效 key、被禁用 key、空 body 的响应必须完全一致。
func TestKeyUsageCreateSessionHidesKeyExistence(t *testing.T) {
	router, _, _ := newTestKeyUsageRouter(t)

	bodies := make([]string, 0, 3)
	for _, payload := range []string{`{"key":"sk-not-a-real-key"}`, `{"key":"sk-disabled-key"}`, `{}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/key-usage/session", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		recorder := doKeyUsageRequest(router, req)
		require.Equal(t, http.StatusUnauthorized, recorder.Code, "payload=%s", payload)
		bodies = append(bodies, recorder.Body.String())
	}
	require.Equal(t, bodies[0], bodies[1])
	require.Equal(t, bodies[0], bodies[2])

	var errBody struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(bodies[0]), &errBody))
	require.NotEmpty(t, errBody.Error.Message)
}

// --- report ----------------------------------------------------------------

func issueTestToken(t *testing.T, svc *service.KeyUsageService, rawKey string) string {
	t.Helper()
	token, _, err := svc.IssueToken(context.Background(), rawKey)
	require.NoError(t, err)
	return token
}

func TestKeyUsageReportContractShape(t *testing.T) {
	router, apiKey, svc := newTestKeyUsageRouter(t)
	token := issueTestToken(t, svc, apiKey.Key)

	recorder := doKeyUsageRequest(router, httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token="+token, nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &raw))
	for _, field := range []string{"key", "usage", "windows", "rankings", "metric", "generated_at"} {
		require.Contains(t, raw, field)
	}

	var body struct {
		Key struct {
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
			Status    string    `json:"status"`
		} `json:"key"`
		Windows map[string]struct {
			Requests int64   `json:"requests"`
			Tokens   int64   `json:"tokens"`
			CostUSD  float64 `json:"cost_usd"`
			Models   []struct {
				Model    string  `json:"model"`
				Requests int64   `json:"requests"`
				Tokens   int64   `json:"tokens"`
				CostUSD  float64 `json:"cost_usd"`
			} `json:"models"`
		} `json:"windows"`
		Rankings map[string]map[string]struct {
			TotalKeys int `json:"total_keys"`
			SelfRank  int `json:"self_rank"`
			Top       []struct {
				Rank     int     `json:"rank"`
				KeyName  string  `json:"key_name"`
				Requests int64   `json:"requests"`
				Tokens   int64   `json:"tokens"`
				CostUSD  float64 `json:"cost_usd"`
				IsSelf   bool    `json:"is_self"`
			} `json:"top"`
			Self struct {
				Rank    int    `json:"rank"`
				KeyName string `json:"key_name"`
				IsSelf  bool   `json:"is_self"`
			} `json:"self"`
		} `json:"rankings"`
		Metric string `json:"metric"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	require.Equal(t, "我的Key", body.Key.Name)
	require.Equal(t, service.StatusAPIKeyActive, body.Key.Status)
	require.Equal(t, "cost", body.Metric)

	for _, window := range []string{"today", "last_7d", "last_30d"} {
		require.Contains(t, body.Windows, window)
		require.Equal(t, int64(3), body.Windows[window].Requests)
		require.Equal(t, int64(900), body.Windows[window].Tokens)
		require.InDelta(t, 1.5, body.Windows[window].CostUSD, 1e-9)
		require.Len(t, body.Windows[window].Models, 1)
	}

	for _, scope := range []string{"account", "site"} {
		require.Contains(t, body.Rankings, scope)
		for _, window := range []string{"today", "last_7d", "last_30d"} {
			ranking := body.Rankings[scope][window]
			require.Equal(t, 2, ranking.TotalKeys)
			require.Len(t, ranking.Top, 2)
			require.Equal(t, 1, ranking.Top[0].Rank)
			require.Equal(t, "别人的Key", ranking.Top[0].KeyName)
			require.False(t, ranking.Top[0].IsSelf)
			require.True(t, ranking.Top[1].IsSelf)
			require.Equal(t, 2, ranking.SelfRank)
			require.Equal(t, "我的Key", ranking.Self.KeyName)
			require.True(t, ranking.Self.IsSelf)
		}
	}

	// top 必须是数组而不是 null（无数据时为空数组）
	require.Contains(t, recorder.Body.String(), `"top":[`)
}

// Bearer 直连与令牌两条路径的响应体必须一致（generated_at 除外）。
func TestKeyUsageReportBearerMatchesTokenPath(t *testing.T) {
	router, apiKey, svc := newTestKeyUsageRouter(t)
	token := issueTestToken(t, svc, apiKey.Key)

	tokenReq := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token="+token, nil)
	bearerReq := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+apiKey.Key)

	tokenBody := decodeKeyUsageBody(t, doKeyUsageRequest(router, tokenReq))
	bearerBody := decodeKeyUsageBody(t, doKeyUsageRequest(router, bearerReq))
	delete(tokenBody, "generated_at")
	delete(bearerBody, "generated_at")
	require.Equal(t, tokenBody, bearerBody)
}

func TestKeyUsageReportMetricSelection(t *testing.T) {
	router, apiKey, svc := newTestKeyUsageRouter(t)
	token := issueTestToken(t, svc, apiKey.Key)

	for _, tc := range []struct{ query, want string }{
		{"", "cost"},
		{"&metric=tokens", "tokens"},
		{"&metric=requests", "requests"},
		{"&metric=bogus", "cost"},
	} {
		recorder := doKeyUsageRequest(router, httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token="+token+tc.query, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		body := decodeKeyUsageBody(t, recorder)
		require.Equal(t, tc.want, body["metric"])
	}
}

func TestKeyUsageReportRejectsBadCredentials(t *testing.T) {
	router, apiKey, svc := newTestKeyUsageRouter(t)
	token := issueTestToken(t, svc, apiKey.Key)

	requests := map[string]*http.Request{
		"no credential":  httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report", nil),
		"garbage token":  httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token=not-a-token", nil),
		"tampered token": httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report?token="+token+"x", nil),
	}
	disabledBearer := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report", nil)
	disabledBearer.Header.Set("Authorization", "Bearer sk-disabled-key")
	requests["disabled key"] = disabledBearer
	unknownBearer := httptest.NewRequest(http.MethodGet, "/api/v1/key-usage/report", nil)
	unknownBearer.Header.Set("Authorization", "Bearer sk-not-a-real-key")
	requests["unknown key"] = unknownBearer

	var bodies []string
	for name, req := range requests {
		recorder := doKeyUsageRequest(router, req)
		require.Equal(t, http.StatusUnauthorized, recorder.Code, name)
		bodies = append(bodies, recorder.Body.String())
	}
	for _, body := range bodies {
		require.Equal(t, bodies[0], body, "所有 401 必须是同一句通用文案")
	}
}

func decodeKeyUsageBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}
