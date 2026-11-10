package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeGrokDelegatedAgentMessagesPreservesTaskAndOrder(t *testing.T) {
	body := []byte(`{"model":"grok-4.5","input":[
		{"type":"message","role":"user","content":"before"},
		{"type":"agent_message","content":[{"type":"input_text","text":"Task: "},{"type":"encrypted_content","encrypted_content":"review code"},{"type":"text","text":" now"}]},
		{"type":"message","role":"user","content":"after"}
	]}`)
	out, err := normalizeGrokDelegatedAgentMessages(body)
	require.NoError(t, err)
	items := gjson.GetBytes(out, "input").Array()
	require.Len(t, items, 3)
	require.Equal(t, "before", items[0].Get("content").String())
	require.Equal(t, "message", items[1].Get("type").String())
	require.Equal(t, "user", items[1].Get("role").String())
	require.Equal(t, "Task: review code now", items[1].Get("content").String())
	require.Equal(t, "after", items[2].Get("content").String())
	require.Equal(t, "grok-4.5", gjson.GetBytes(out, "model").String())
}

func TestNormalizeGrokDelegatedAgentMessagesDropsEmptyEnvelope(t *testing.T) {
	body := []byte(`{"input":[{"type":"agent_message","content":[{"type":"unknown","text":"discard"}]},{"type":"message","role":"user","content":"keep"}]}`)
	out, err := normalizeGrokDelegatedAgentMessages(body)
	require.NoError(t, err)
	items := gjson.GetBytes(out, "input").Array()
	require.Len(t, items, 1)
	require.Equal(t, "keep", items[0].Get("content").String())
}

func TestNormalizeGrokDelegatedAgentMessagesKeepsUnrelatedInputsUnchanged(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"input":[{"type":"message","role":"user","content":"hello"}]}`),
		[]byte(`{"input":"unchanged"}`),
		[]byte(`{"input":[{"type":"agent_message","content":"direct text"}]}`),
	} {
		out, err := normalizeGrokDelegatedAgentMessages(body)
		require.NoError(t, err)
		if gjson.GetBytes(body, "input.0.type").String() == "agent_message" {
			require.Equal(t, "direct text", gjson.GetBytes(out, "input.0.content").String())
		} else {
			require.Equal(t, string(body), string(out))
		}
	}
}
