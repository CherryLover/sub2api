package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newUsageEndpointRouter 端到端挂载 GET /v1/usage：鉴权中间件已由 API Key 中间件完成，
// 这里用一个只写 context 的假中间件替代，其余（状态码、响应体形状）全部走真实路径。
//
// 只用私有方法（usageUnrestricted 等）做单测会丢掉状态码与响应形状这两个契约，
// 而 /v1/usage 是 CC Switch 等外部客户端直接消费的接口，必须有端到端断言兜底。
func newUsageEndpointRouter(h *GatewayHandler, apiKey *service.APIKey, subject *middleware.AuthSubject) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/usage", func(c *gin.Context) {
		if apiKey != nil {
			c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		}
		if subject != nil {
			c.Set(string(middleware.ContextKeyUser), *subject)
		}
		c.Next()
	}, h.Usage)
	return router
}

func doUsageRequest(router *gin.Engine, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// quota_limited 模式的端到端契约：200 + JSON + 额度字段齐全。
func TestUsageEndpointQuotaLimitedContract(t *testing.T) {
	apiKey := &service.APIKey{
		ID:        3,
		UserID:    7,
		Status:    service.StatusAPIKeyActive,
		Quota:     100,
		QuotaUsed: 40,
	}
	router := newUsageEndpointRouter(&GatewayHandler{}, apiKey, &middleware.AuthSubject{UserID: 7}, nil)

	recorder := doUsageRequest(router, "/v1/usage")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

	var body struct {
		Mode    string `json:"mode"`
		IsValid bool   `json:"isValid"`
		Status  string `json:"status"`
		Quota   struct {
			Limit     float64 `json:"limit"`
			Used      float64 `json:"used"`
			Remaining float64 `json:"remaining"`
			Unit      string  `json:"unit"`
		} `json:"quota"`
		Remaining float64 `json:"remaining"`
		Unit      string  `json:"unit"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "quota_limited", body.Mode)
	require.True(t, body.IsValid)
	require.Equal(t, service.StatusAPIKeyActive, body.Status)
	require.InDelta(t, 100, body.Quota.Limit, 1e-9)
	require.InDelta(t, 40, body.Quota.Used, 1e-9)
	require.InDelta(t, 60, body.Quota.Remaining, 1e-9)
	require.Equal(t, "USD", body.Quota.Unit)
	require.InDelta(t, 60, body.Remaining, 1e-9)
	require.Equal(t, "USD", body.Unit)
}

// unrestricted 模式的端到端契约：Key 未配额度与速率限制时按"不限额度"返回。
func TestUsageEndpointUnrestrictedContract(t *testing.T) {
	apiKey := &service.APIKey{
		ID:     4,
		UserID: 7,
		Status: service.StatusAPIKeyActive,
		Group: &service.Group{
			Name: "Default group",
		},
	}
	router := newUsageEndpointRouter(
		&GatewayHandler{},
		apiKey,
		&middleware.AuthSubject{UserID: 7},
	)

	recorder := doUsageRequest(router, "/v1/usage")
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Mode      string   `json:"mode"`
		PlanName  string   `json:"planName"`
		Unit      string   `json:"unit"`
		Remaining *float64 `json:"remaining"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "unrestricted", response.Mode)
	require.Equal(t, "Default group", response.PlanName)
	require.Equal(t, "USD", response.Unit)
	require.NotNil(t, response.Remaining)
	require.Equal(t, float64(-1), *response.Remaining)
}

// 缺鉴权上下文时必须是 401，而不是空 200。
func TestUsageEndpointRequiresAuthenticatedContext(t *testing.T) {
	router := newUsageEndpointRouter(&GatewayHandler{}, nil, nil)
	require.Equal(t, http.StatusUnauthorized, doUsageRequest(router, "/v1/usage").Code)

	// 有 API Key 但没有 AuthSubject 同样是 401。
	router = newUsageEndpointRouter(&GatewayHandler{}, &service.APIKey{ID: 1, Quota: 10}, nil)
	require.Equal(t, http.StatusUnauthorized, doUsageRequest(router, "/v1/usage").Code)
}

// days 超出 1-90 时是 400，且带上可读的错误信息。
func TestUsageEndpointRejectsOutOfRangeDays(t *testing.T) {
	router := newUsageEndpointRouter(
		&GatewayHandler{},
		&service.APIKey{ID: 3, UserID: 7, Status: service.StatusAPIKeyActive, Quota: 100},
		&middleware.AuthSubject{UserID: 7},
	)

	recorder := doUsageRequest(router, "/v1/usage?days=999")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "1-90")
}

// 保留一层直接调用，锁住 payload 组装本身（不经过 HTTP 层）。
func TestUsageUnrestrictedUsesGroupNameAndUnlimitedRemaining(t *testing.T) {
	handler := &GatewayHandler{}
	payload := handler.usageUnrestricted(
		&service.APIKey{Group: &service.Group{Name: "Default group"}},
		nil,
		nil,
		nil,
	)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	var response struct {
		Mode      string  `json:"mode"`
		PlanName  string  `json:"planName"`
		Remaining float64 `json:"remaining"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	require.Equal(t, "unrestricted", response.Mode)
	require.Equal(t, "Default group", response.PlanName)
	require.Equal(t, float64(-1), response.Remaining)
}
