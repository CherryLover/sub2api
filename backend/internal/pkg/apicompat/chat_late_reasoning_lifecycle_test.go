package apicompat

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChatLateReasoningPreservesAnthropicBlockLifecycle(t *testing.T) {
	text := `{"choices":[{"index":0,"delta":{"content":"visible"}}]}`
	reason := `{"choices":[{"index":0,"delta":{"reasoning_content":"plan"}}]}`
	tool := `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}}]}`
	for _, tc := range []struct {
		name   string
		chunks []string
	}{
		{"text_reasoning_tool", []string{text, reason, tool}},
		{"reasoning_text_tool", []string{reason, text, tool}},
		{"text_tool", []string{text, tool}},
		{"text_reasoning_text_tool", []string{text, reason, text, tool}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses := NewChatCompletionsToResponsesStreamState("fixture-model")
			anthropic := NewResponsesEventToAnthropicState()
			var events []AnthropicStreamEvent
			consume := func(input []ResponsesStreamEvent) {
				for i := range input {
					events = append(events, ResponsesEventToAnthropicEvents(&input[i], anthropic)...)
				}
			}
			for _, raw := range tc.chunks {
				var chunk ChatCompletionsChunk
				require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
				consume(ChatCompletionsChunkToResponsesEvents(&chunk, responses))
			}
			consume(FinalizeChatCompletionsResponsesStream(responses))
			live := map[int]string{}
			seen := map[int]bool{}
			stopped := false
			for _, event := range events {
				switch event.Type {
				case "content_block_start":
					require.False(t, stopped)
					require.NotNil(t, event.Index)
					require.NotNil(t, event.ContentBlock)
					require.False(t, seen[*event.Index], "content block index was reused")
					seen[*event.Index] = true
					live[*event.Index] = event.ContentBlock.Type
				case "content_block_delta":
					require.False(t, stopped)
					require.NotNil(t, event.Index)
					require.NotNil(t, event.Delta)
					kind, ok := live[*event.Index]
					require.True(t, ok, "%s references closed/unstarted index %d", event.Delta.Type, *event.Index)
					want := map[string]string{"text_delta": "text", "thinking_delta": "thinking", "signature_delta": "thinking", "input_json_delta": "tool_use"}[event.Delta.Type]
					require.Equal(t, want, kind)
				case "content_block_stop":
					require.NotNil(t, event.Index)
					_, ok := live[*event.Index]
					require.True(t, ok, "stop without live block %d", *event.Index)
					delete(live, *event.Index)
				case "message_stop":
					require.Empty(t, live)
					require.False(t, stopped)
					stopped = true
				}
			}
			require.True(t, stopped)
			require.Empty(t, live)
		})
	}
}
