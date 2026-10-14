package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertedResponsesInputStringRejectionSafety(t *testing.T) {
	accepted := `{"error":{"type":"invalid_request_error","code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`
	tests := []struct {
		name, response string
		status         int
		want           bool
	}{
		{"positive", accepted, http.StatusBadRequest, true},
		{"wrong status", accepted, http.StatusTooManyRequests, false},
		{"wrong parameter", strings.Replace(accepted, `"param":"input"`, `"param":"tools"`, 1), http.StatusBadRequest, false},
		{"auth error", strings.Replace(accepted, `"type":"invalid_request_error"`, `"type":"authentication_error"`, 1), http.StatusBadRequest, false},
		{"duplicate code", strings.Replace(accepted, `"code":"invalid_type"`, `"code":"invalid_type","code":"rate_limit_exceeded"`, 1), http.StatusBadRequest, false},
		{"partial data", strings.TrimSuffix(accepted, "}") + `,"usage":{"input_tokens":5}}`, http.StatusBadRequest, false},
		{"malformed", `{"error":`, http.StatusBadRequest, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isConvertedResponsesInputStringRejection(tc.status, []byte(tc.response)))
		})
	}
}

func TestAutoResponsesStructuredInputRetryPreservesChatHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"call1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call1","content":"done"}],"stream":false,"vendor_option":"retain"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_type","param":"input","message":"Expected a string, but got an array instead."}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl_ok","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}}, httpUpstream: upstream}
	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"}, Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: true}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), ctx, account, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "input").IsArray())
	require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
	require.JSONEq(t, string(body), string(upstream.bodies[1]))
	require.Equal(t, "/v1/chat/completions", result.UpstreamEndpoint)
	require.Equal(t, "ok", gjson.Get(recorder.Body.String(), "choices.0.message.content").String())
}
