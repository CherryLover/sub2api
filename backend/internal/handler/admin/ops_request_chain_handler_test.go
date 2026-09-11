//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const opsRequestChainRoute = "/api/v1/admin/ops/requests/:clientRequestId/chain"

// opsRequestChainRepoStub 只实现链路查询这一个可选能力。
type opsRequestChainRepoStub struct {
	service.OpsRepository
	source *service.OpsRequestChainSource
	gotID  string
	calls  int
}

func (r *opsRequestChainRepoStub) GetRequestChainSource(_ context.Context, clientRequestID string) (*service.OpsRequestChainSource, error) {
	r.calls++
	r.gotID = clientRequestID
	return r.source, nil
}

func newOpsRequestChainRouter(ops *service.OpsService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(opsRequestChainRoute, NewOpsHandler(ops).GetRequestChain)
	return router
}

func getOpsRequestChain(router *gin.Engine, clientRequestID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/requests/"+clientRequestID+"/chain", nil)
	router.ServeHTTP(rec, req)
	return rec
}

// 接口契约以 JSON 字段名为准，所以解到 map 里断言，而不是解回 Go 结构体。
func decodeOpsRequestChain(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	require.Equal(t, 0, env.Code)
	require.NotNil(t, env.Data)
	return env.Data
}

func TestOpsRequestChainHandler_ReturnsDocumentedShape(t *testing.T) {
	at := time.Date(2026, 9, 11, 6, 59, 39, 0, time.UTC)
	ttft := int64(18501)
	repo := &opsRequestChainRepoStub{source: &service.OpsRequestChainSource{
		Errors: []*service.OpsRequestChainErrorRecord{{
			ID:             1,
			CreatedAt:      at,
			StatusCode:     200,
			Model:          "gpt-5.6-sol",
			RequestedModel: "gpt-5.6-sol",
			Stream:         true,
			Attempts: []*service.OpsRequestChainAttemptRecord{{
				AtUnixMs:           at.UnixMilli(),
				AccountID:          7,
				AccountName:        "anviz-7",
				Platform:           "openai",
				UpstreamStatusCode: 502,
				Message:            "Our servers are currently overloaded.",
				Kind:               "failover",
			}},
			TimeToFirstTokenMs: &ttft,
		}},
		Usage: &service.OpsRequestChainUsageRecord{
			AccountID: 7, AccountName: "anviz-7", TotalTokens: 1883, Cost: 0.0212,
		},
	}}
	router := newOpsRequestChainRouter(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	data := decodeOpsRequestChain(t, getOpsRequestChain(router, "req-abc"))

	require.Equal(t, "req-abc", data["client_request_id"])
	require.Equal(t, "req-abc", repo.gotID)
	require.Equal(t, "gpt-5.6-sol", data["model"])
	require.Equal(t, "gpt-5.6-sol", data["requested_model"])
	require.Equal(t, true, data["stream"])
	require.Equal(t, "recovered", data["outcome"])
	require.EqualValues(t, 200, data["client_status_code"])

	attempts, ok := data["attempts"].([]any)
	require.True(t, ok, "attempts 必须是数组：%#v", data["attempts"])
	require.Len(t, attempts, 1)
	attempt, ok := attempts[0].(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 1, attempt["seq"])
	require.EqualValues(t, 7, attempt["account_id"])
	require.Equal(t, "anviz-7", attempt["account_name"])
	require.Equal(t, "openai", attempt["platform"])
	require.EqualValues(t, 502, attempt["upstream_status_code"])
	require.Equal(t, "failover", attempt["kind"])
	require.Equal(t, "Our servers are currently overloaded.", attempt["message"])
	rawAt, ok := attempt["at"].(string)
	require.True(t, ok, "at 必须是字符串：%#v", attempt["at"])
	attemptAt, err := time.Parse(time.RFC3339Nano, rawAt)
	require.NoError(t, err)
	require.True(t, at.Equal(attemptAt))

	final, ok := data["final"].(map[string]any)
	require.True(t, ok, "final 必须是对象：%#v", data["final"])
	require.EqualValues(t, 7, final["account_id"])
	require.Equal(t, true, final["succeeded"])
	require.EqualValues(t, 18501, final["time_to_first_token_ms"])
	require.EqualValues(t, 1883, final["total_tokens"])
	require.InDelta(t, 0.0212, final["cost"], 1e-9)
}

// 关联不到 usage_logs 时 final 必须是 null，不是被省略，也不是编出来的对象。
func TestOpsRequestChainHandler_FinalNullWithoutUsageLog(t *testing.T) {
	repo := &opsRequestChainRepoStub{source: &service.OpsRequestChainSource{
		Errors: []*service.OpsRequestChainErrorRecord{{
			ID:         2,
			CreatedAt:  time.Now().UTC(),
			StatusCode: 502,
		}},
	}}
	router := newOpsRequestChainRouter(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	data := decodeOpsRequestChain(t, getOpsRequestChain(router, "req-failed"))
	require.Equal(t, "failed", data["outcome"])
	require.Contains(t, data, "final")
	require.Nil(t, data["final"])
	attempts, ok := data["attempts"].([]any)
	require.True(t, ok, "upstream_errors 为空时 attempts 仍是数组")
	require.Empty(t, attempts)
}

func TestOpsRequestChainHandler_UnknownRequestReturns404(t *testing.T) {
	repo := &opsRequestChainRepoStub{source: &service.OpsRequestChainSource{}}
	router := newOpsRequestChainRouter(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	rec := getOpsRequestChain(router, "req-missing")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, 1, repo.calls)
}

func TestOpsRequestChainHandler_InvalidIDReturns400(t *testing.T) {
	repo := &opsRequestChainRepoStub{source: &service.OpsRequestChainSource{}}
	router := newOpsRequestChainRouter(service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	rec := getOpsRequestChain(router, strings.Repeat("x", opsRequestChainMaxIDLen+1))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Zero(t, repo.calls)
}

func TestOpsRequestChainHandler_OpsDisabledDoesNotQuery(t *testing.T) {
	repo := &opsRequestChainRepoStub{source: &service.OpsRequestChainSource{}}
	ops := service.NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: false}}, nil, nil, nil, nil, nil, nil, nil, nil)

	rec := getOpsRequestChain(newOpsRequestChainRouter(ops), "req-abc")
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.Zero(t, repo.calls)
}

func TestOpsRequestChainHandler_NilServiceReturns503(t *testing.T) {
	rec := getOpsRequestChain(newOpsRequestChainRouter(nil), "req-abc")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
