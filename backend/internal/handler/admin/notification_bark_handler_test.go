package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type barkHandlerTestEncryptor struct{}

func (barkHandlerTestEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (barkHandlerTestEncryptor) Decrypt(ciphertext string) (string, error) {
	rest, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", errors.New("not encrypted")
	}
	return rest, nil
}

type barkHandlerTestSender struct {
	sendErr error
	sent    int
}

func (s *barkHandlerTestSender) Send(context.Context, service.BarkTarget, service.BarkMessage) (*service.BarkSendResult, error) {
	s.sent++
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return &service.BarkSendResult{StatusCode: http.StatusOK, Message: "success"}, nil
}

func (s *barkHandlerTestSender) Ping(context.Context, string) error { return nil }

func newBarkHandlerRouter(t *testing.T, sender *barkHandlerTestSender) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := service.NewBarkNotificationService(newTestSettingRepo(), barkHandlerTestEncryptor{}, sender, true)
	h := NewNotificationBarkHandler(svc)

	r := gin.New()
	r.GET("/api/v1/admin/notifications/bark", h.GetBarkConfig)
	r.PUT("/api/v1/admin/notifications/bark", h.UpdateBarkConfig)
	r.POST("/api/v1/admin/notifications/bark/test", h.TestBark)
	return r
}

func doBarkJSON(t *testing.T, r *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, apiEnvelope) {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	var envelope apiEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope), rec.Body.String())
	return rec, envelope
}

func TestNotificationBarkHandler_GetDefaultShape(t *testing.T) {
	r := newBarkHandlerRouter(t, &barkHandlerTestSender{})
	rec, envelope := doBarkJSON(t, r, http.MethodGet, "/api/v1/admin/notifications/bark", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.JSONEq(t, `false`, string(data["enabled"]))
	require.JSONEq(t, `""`, string(data["server_url"]))
	require.JSONEq(t, `""`, string(data["device_key"]))
	require.JSONEq(t, `false`, string(data["has_device_key"]))
	require.JSONEq(t, `"sub2api"`, string(data["group"]))
	require.JSONEq(t, `"active"`, string(data["level"]))
	require.JSONEq(t, `""`, string(data["sound"]))
	require.JSONEq(t, `""`, string(data["click_url"]))
	require.JSONEq(t, `true`, string(data["notify_on_resolve"]))
	require.NotContains(t, data, "updated_at")
}

func TestNotificationBarkHandler_PutThenGetMasksKey(t *testing.T) {
	r := newBarkHandlerRouter(t, &barkHandlerTestSender{})
	rec, envelope := doBarkJSON(t, r, http.MethodPut, "/api/v1/admin/notifications/bark",
		`{"enabled":true,"server_url":"https://api.day.app/","device_key":"abc","group":"sub2api","level":"timeSensitive","sound":"","click_url":"","notify_on_resolve":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.JSONEq(t, `true`, string(data["enabled"]))
	require.JSONEq(t, `"https://api.day.app"`, string(data["server_url"]))
	require.JSONEq(t, `""`, string(data["device_key"]))
	require.JSONEq(t, `true`, string(data["has_device_key"]))
	require.JSONEq(t, `"timeSensitive"`, string(data["level"]))
	require.Contains(t, data, "updated_at")
	require.NotContains(t, string(envelope.Data), "abc")

	rec, envelope = doBarkJSON(t, r, http.MethodGet, "/api/v1/admin/notifications/bark", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, string(envelope.Data), "abc")
	require.Contains(t, string(envelope.Data), `"has_device_key":true`)
}

func TestNotificationBarkHandler_PutValidationErrors(t *testing.T) {
	r := newBarkHandlerRouter(t, &barkHandlerTestSender{})

	rec, envelope := doBarkJSON(t, r, http.MethodPut, "/api/v1/admin/notifications/bark",
		`{"enabled":true,"server_url":"https://api.day.app","device_key":"abc","level":"urgent"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "BARK_LEVEL_INVALID", envelope.Reason)

	rec, envelope = doBarkJSON(t, r, http.MethodPut, "/api/v1/admin/notifications/bark",
		`{"enabled":true,"server_url":"https://api.day.app","device_key":""}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "BARK_DEVICE_KEY_REQUIRED", envelope.Reason)
	require.Contains(t, envelope.Message, "device_key")

	rec, envelope = doBarkJSON(t, r, http.MethodPut, "/api/v1/admin/notifications/bark",
		`{"enabled":true,"server_url":"api.day.app","device_key":"abc"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "BARK_SERVER_URL_INVALID", envelope.Reason)

	rec, _ = doBarkJSON(t, r, http.MethodPut, "/api/v1/admin/notifications/bark", `{"enabled":"yes"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNotificationBarkHandler_TestEndpoint(t *testing.T) {
	sender := &barkHandlerTestSender{}
	r := newBarkHandlerRouter(t, sender)

	var data struct {
		OK         bool   `json:"ok"`
		PingOK     bool   `json:"ping_ok"`
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		LatencyMs  int64  `json:"latency_ms"`
	}

	// server_url 缺失 → 400。
	rec, envelope := doBarkJSON(t, r, http.MethodPost, "/api/v1/admin/notifications/bark/test", `{"device_key":"abc"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "BARK_SERVER_URL_INVALID", envelope.Reason)
	require.Equal(t, 0, sender.sent)

	// 没有任何 key → 200，只探活不推送：ok=false、ping_ok 为探活结果、status_code=0。
	rec, envelope = doBarkJSON(t, r, http.MethodPost, "/api/v1/admin/notifications/bark/test",
		`{"server_url":"https://api.day.app"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.False(t, data.OK)
	require.True(t, data.PingOK)
	require.Equal(t, 0, data.StatusCode)
	require.Equal(t, "未配置设备 Key，仅测试了服务器连通性", data.Message)
	require.GreaterOrEqual(t, data.LatencyMs, int64(0))
	require.Equal(t, 0, sender.sent)

	// 请求里带 key → 推送成功。
	rec, envelope = doBarkJSON(t, r, http.MethodPost, "/api/v1/admin/notifications/bark/test",
		`{"server_url":"https://api.day.app","device_key":"abc","title":"hi","body":"there"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.True(t, data.OK)
	require.True(t, data.PingOK)
	require.Equal(t, http.StatusOK, data.StatusCode)
	require.Equal(t, "success", data.Message)
	require.Equal(t, 1, sender.sent)

	// 上游拒绝 → 502，错误里带状态码与响应片段。
	sender.sendErr = &service.BarkSendError{StatusCode: http.StatusBadRequest, Snippet: `{"code":400,"message":"device key is invalid"}`}
	rec, envelope = doBarkJSON(t, r, http.MethodPost, "/api/v1/admin/notifications/bark/test",
		`{"server_url":"https://api.day.app","device_key":"abc"}`)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, "BARK_PUSH_FAILED", envelope.Reason)
	require.Contains(t, envelope.Message, "400")
	require.Contains(t, envelope.Message, "device key is invalid")
}

func TestNotificationBarkHandler_BlankGroupFallsBackToDefault(t *testing.T) {
	r := newBarkHandlerRouter(t, &barkHandlerTestSender{})
	rec, envelope := doBarkJSON(t, r, http.MethodPut, "/api/v1/admin/notifications/bark",
		`{"enabled":false,"server_url":"https://api.day.app","group":"   "}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, string(envelope.Data), `"group":"sub2api"`)

	rec, envelope = doBarkJSON(t, r, http.MethodGet, "/api/v1/admin/notifications/bark", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, string(envelope.Data), `"group":"sub2api"`)
}
