package apicompat

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNormalizeResponsesLeadingChatInstructionsPreservesPriority(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: json.RawMessage(`"System policy"`)},
		{Role: "developer", Content: json.RawMessage(`"Developer rules"`)},
		{Role: "user", Content: json.RawMessage(`"Question"`)},
		{Role: "developer", Content: json.RawMessage(`"Later developer notice"`)},
	}
	out := normalizeResponsesLeadingChatInstructions(msgs)
	require.Len(t, out, 3)
	require.Equal(t, "system", out[0].Role)
	require.Equal(t, "System policy\n\nDeveloper rules", chatMessageContentText(out[0].Content))
	require.Equal(t, "user", out[1].Role)
	require.Equal(t, "developer", out[2].Role, "must not demote a later developer instruction")
	require.Len(t, msgs, 4, "original input must remain intact")
}
func TestNormalizeResponsesLeadingChatInstructionsSkipsUnstructuredData(t *testing.T) {
	messages := []ChatMessage{{Role: "system", Content: json.RawMessage(`{"unexpected":"object"}`)}, {Role: "developer", Content: json.RawMessage(`"rules"`)}}
	require.Equal(t, messages, normalizeResponsesLeadingChatInstructions(messages))
}
