package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Haiku 5.5 signed-thinking support from the merged main branch must coexist
// with the remaining backport's inline system-message conversion.
func TestHaiku55InlineSystemPreservesSignedThinking(t *testing.T) {
	block := AnthropicContentBlock{Type: "thinking", Thinking: "", Signature: "signed-haiku-inline"}
	response := AnthropicToResponsesResponse(&AnthropicResponse{
		Model: "claude-haiku-5-5",
		Content: []AnthropicContentBlock{block, {Type: "text", Text: "progress"},
			{Type: "tool_use", ID: "toolu_inline", Name: "lookup", Input: json.RawMessage(`{}`)}},
	})
	raw, err := json.Marshal(response.Output)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(raw, &items))
	items = append(items,
		ResponsesInputItem{Type: "function_call_output", CallID: "toolu_inline", Output: "ok"},
		ResponsesInputItem{Role: "developer", Content: json.RawMessage(`"new policy"`)},
	)
	raw, err = json.Marshal(items)
	require.NoError(t, err)
	out, err := ResponsesToAnthropicRequestWithOptions(&ResponsesRequest{
		Model: "claude-haiku-5-5", Instructions: "stable policy", Input: raw,
	}, ResponsesAnthropicOptions{MidConversationSystem: true})
	require.NoError(t, err)
	require.JSONEq(t, `"stable policy"`, string(out.System))
	require.Len(t, out.Messages, 3)
	require.Equal(t, "assistant", out.Messages[0].Role)
	require.Equal(t, "user", out.Messages[1].Role)
	require.Equal(t, "system", out.Messages[2].Role)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(out.Messages[0].Content, &blocks))
	require.Equal(t, block, blocks[0])
	require.Equal(t, "new policy", parseContentBlocks(out.Messages[2].Content)[0].Text)
	require.Equal(t, "adaptive", out.Thinking.Type)
}
