//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardAsRawChatCompletions_EmptyStreamTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-empty"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}}}
	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, OpenAIUpstreamStreamTruncatedCode, gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
	require.False(t, c.Writer.Written())
}

func TestForwardAsRawChatCompletions_TruncatedStreamAfterOutputReturnsTypedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstreamBody := "data: {\"id\":\"chatcmpl_cut\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"half\"},\"finish_reason\":null}]}\n\n"
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}}
	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
	require.NotNil(t, result)
	code, _, ok := OpenAIUpstreamStreamReadErrorDetails(err)
	require.True(t, ok)
	require.Equal(t, OpenAIUpstreamStreamTruncatedCode, code)
	require.Contains(t, rec.Body.String(), `"content":"half"`)
}

func TestOpenAIRawStreamTerminalState(t *testing.T) {
	for _, tt := range []struct {
		name, payload string
		terminated    bool
	}{
		{"done", "[DONE]", true},
		{"usage", `{"choices":[],"usage":{"prompt_tokens":1}}`, true},
		{"finish reason", `{"choices":[{"finish_reason":"stop"}]}`, true},
		{"nonterminal", `{"choices":[{"finish_reason":null}]}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var state openAIRawStreamTerminalState
			state.ObserveDataLine(tt.payload)
			require.Equal(t, tt.terminated, state.Terminated())
		})
	}
	var empty openAIRawStreamTerminalState
	require.True(t, empty.IsTruncated(false))
	require.False(t, empty.IsTruncated(true), "non-SSE bodies retain the existing raw forwarding behavior")
}
