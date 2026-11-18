package apicompat

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChatAssistantPriorReasoningIsNotVisibleOutput(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		message                            ChatMessage
		expectedReasoning, expectedVisible string
	}{
		{"reasoning field", ChatMessage{Role: "assistant", ReasoningContent: "private reasoning", Content: json.RawMessage(`"final answer"`)}, "private reasoning", "final answer"},
		{"structured thinking", ChatMessage{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"private reasoning"},{"type":"text","text":"final answer"}]`)}, "private reasoning", "final answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, err := chatAssistantToResponses(tc.message)
			require.NoError(t, err)
			require.Len(t, items, 2)
			require.Equal(t, "reasoning", items[0].Type)
			require.JSONEq(t, `[{"type":"summary_text","text":"private reasoning"}]`, string(items[0].Summary))
			require.Equal(t, "message", items[1].Type)
			require.NotContains(t, string(items[1].Content), "private reasoning")
			require.NotContains(t, string(items[1].Content), "<thinking>")
			require.Contains(t, string(items[1].Content), "final answer")
		})
	}
}
