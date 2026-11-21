package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAntigravityResponsesCustomToolRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gemini-3.1-pro-high","stream":%v,"tools":[{"type":"custom","name":"apply_patch","format":{"type":"text"}}],"input":[{"type":"message","role":"user","content":"apply patch"},{"type":"custom_tool_call","name":"apply_patch","call_id":"call_old","input":"old patch"},{"type":"custom_tool_call_output","call_id":"call_old","output":"done"}]}`, stream))
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"response\":{\"responseId\":\"ag-tool\",\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"apply_patch\",\"args\":{\"input\":\"next patch\"}}}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3}}}\n\n"))}}
			svc := newAntigravityCompatService(config.GatewayConfig{}, upstream)
			c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
			result, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 8, result.Usage.InputTokens)
			require.Contains(t, rec.Body.String(), `custom_tool_call`)
			require.Contains(t, rec.Body.String(), `next patch`)
			found := false
			gjson.GetBytes(upstream.lastBody, "request.contents").ForEach(func(_, message gjson.Result) bool {
				message.Get("parts").ForEach(func(_, part gjson.Result) bool {
					if part.Get("functionResponse.name").String() == "apply_patch" {
						found = true
					}
					return true
				})
				return true
			})
			require.True(t, found, "custom tool result must not disappear from history")
		})
	}
}

// A client-only tool cannot make trailing garbage valid or silently collapse
// two conflicting tools into an ambiguous Gemini function invocation.
func TestAntigravityResponsesCustomToolInvalidBodyAndConflictFailClosed(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"trailing document", `{"model":"gemini-3.1-pro-high","tools":[{"type":"custom","name":"apply_patch","format":{"type":"text"}}],"input":"hi"} {"extra":true}`},
		{"custom and function name collide", `{"model":"gemini-3.1-pro-high","tools":[{"type":"custom","name":"apply_patch"},{"type":"function","name":"apply_patch","parameters":{"type":"object"}}],"input":"hi"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			svc := newAntigravityCompatService(config.GatewayConfig{}, upstream)
			body := []byte(tc.body)
			c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
			_, err := svc.ForwardAsResponses(context.Background(), c, newAntigravityCompatAccount(AccountTypeOAuth), body, nil)
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Nil(t, upstream.lastReq, "invalid client tools must never reach an upstream account")
		})
	}
}
