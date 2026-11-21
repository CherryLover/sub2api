package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesBridgeRepairsInterruptedAssistantHistoryOnMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"claude-opus-5-5", "claude-fable-5-2"} {
		for _, stream := range []bool{false, true} {
			for _, scenario := range []string{"commentary", "unanswered-tool", "thinking-only", "answered-tool"} {
				t.Run(fmt.Sprintf("%s/stream=%v/%s", model, stream, scenario), func(t *testing.T) {
					blocks := []apicompat.AnthropicContentBlock{{Type: "thinking", Thinking: "private plan", Signature: "signed-fixture"}}
					if scenario == "commentary" || scenario == "unanswered-tool" {
						blocks = append(blocks, apicompat.AnthropicContentBlock{Type: "text", Text: "Checking now."})
					}
					if scenario != "commentary" {
						blocks = append(blocks, apicompat.AnthropicContentBlock{Type: "tool_use", ID: "call_lookup", Name: "lookup", Input: json.RawMessage(`{"value":1}`)})
					}
					history := apicompat.AnthropicToResponsesResponse(&apicompat.AnthropicResponse{Model: model, Content: blocks})
					raw, err := json.Marshal(history.Output)
					require.NoError(t, err)
					var items []json.RawMessage
					require.NoError(t, json.Unmarshal(raw, &items))
					items = append([]json.RawMessage{json.RawMessage(`{"type":"message","role":"user","content":"Start."}`)}, items...)
					if scenario == "answered-tool" {
						items = append(items, json.RawMessage(`{"type":"function_call_output","call_id":"call_lookup","output":"found"}`))
					}
					input, err := json.Marshal(items)
					require.NoError(t, err)
					body, err := json.Marshal(map[string]any{"model": "public-claude", "input": json.RawMessage(input), "stream": stream})
					require.NoError(t, err)
					upstreamBody := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_continue\",\"model\":\"" + model + "\",\"content\":[],\"usage\":{\"input_tokens\":10}}}\n\n" +
						"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"continued\"}}\n\n" +
						"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}}
					svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream}
					account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key", "model_mapping": map[string]any{"public-claude": model}}}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
					result, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 200, rec.Code)
					require.Equal(t, 10, result.Usage.InputTokens)
					require.Equal(t, 5, result.Usage.OutputTokens)
					require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
					messages := gjson.GetBytes(upstream.lastBody, "messages").Array()
					require.NotEmpty(t, messages)
					require.Equal(t, "user", messages[len(messages)-1].Get("role").String())
					for _, message := range messages {
						if message.Get("role").String() != "assistant" {
							continue
						}
						parts := message.Get("content").Array()
						require.NotEmpty(t, parts)
						require.NotContains(t, []string{"thinking", "redacted_thinking"}, parts[len(parts)-1].Get("type").String())
					}
					if scenario == "answered-tool" {
						require.Contains(t, string(upstream.lastBody), "signed-fixture")
						require.Contains(t, string(upstream.lastBody), `"tool_use"`)
						require.Contains(t, string(upstream.lastBody), `"tool_result"`)
						require.NotContains(t, string(upstream.lastBody), "Continue.")
					}
					if scenario == "thinking-only" {
						require.NotContains(t, string(upstream.lastBody), "signed-fixture")
					}
				})
			}
		}
	}
}
