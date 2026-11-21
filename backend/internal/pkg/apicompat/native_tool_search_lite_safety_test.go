package apicompat

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNativeToolSearchRepeatedDoneExecutesOnce(t *testing.T) {
	item := &ResponsesOutput{Type: "tool_search_call", CallID: "call_s", Status: "completed", Arguments: `{"query":"lookup"}`}
	event := &ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: item}
	state := NewResponsesEventToAnthropicState()
	first := ResponsesEventToAnthropicEvents(event, state)
	second := ResponsesEventToAnthropicEvents(event, state)
	require.NotEmpty(t, first)
	require.Empty(t, second)
	accumulator := NewBufferedResponseAccumulator()
	accumulator.ProcessEvent(event)
	accumulator.ProcessEvent(event)
	output := accumulator.BuildOutput()
	require.Len(t, output, 1)
	require.Equal(t, "call_s", output[0].CallID)
}
func TestNativeToolSearchFailedDoneDoesNotExecute(t *testing.T) {
	for _, status := range []string{"failed", "in_progress", "incomplete"} {
		event := &ResponsesStreamEvent{Type: "response.output_item.done", Item: &ResponsesOutput{Type: "tool_search_call", CallID: "call_s", Status: status, Arguments: `{}`}}
		state := NewResponsesEventToAnthropicState()
		events := ResponsesEventToAnthropicEvents(event, state)
		for _, e := range events {
			require.NotEqual(t, "content_block_start", e.Type)
		}
		accumulator := NewBufferedResponseAccumulator()
		accumulator.ProcessEvent(event)
		require.False(t, accumulator.HasContent())
	}
}
func TestNativeToolSearchResultTextKeepsUnknownUserFields(t *testing.T) {
	raw := json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"existing"}],"opaque":{"integer":9223372036854775807}}`)
	result, err := userMessageWithLeadingText(raw, "search failed")
	require.NoError(t, err)
	require.Contains(t, string(result), `9223372036854775807`)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &fields))
	require.JSONEq(t, `{"integer":9223372036854775807}`, string(fields["opaque"]))
	require.JSONEq(t, `[{"type":"input_text","text":"search failed"},{"type":"input_text","text":"existing"}]`, string(fields["content"]))
}
