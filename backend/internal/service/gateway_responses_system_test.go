//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestResponsesInlineSystemAccountCapability(t *testing.T) {
	input := json.RawMessage(`[{"role":"user","content":"q"},{"role":"developer","content":"policy"}]`)
	for _, tc := range []struct {
		name, model, typ string
		disabled, inline bool
	}{
		{"opus 5.5 api key", "claude-opus-5-5", AccountTypeAPIKey, false, true},
		{"sonnet 5.5 api key", "claude-sonnet-5-5", AccountTypeAPIKey, false, true},
		{"vendor prefix", "vendor/claude-opus-5-5", AccountTypeAPIKey, false, true},
		{"claude 4 legacy", "claude-sonnet-4-5", AccountTypeAPIKey, false, false},
		{"non-Claude", "gpt-5", AccountTypeAPIKey, false, false},
		{"oauth not supported", "claude-opus-5-5", AccountTypeOAuth, false, false},
		{"account opted out", "claude-opus-5-5", AccountTypeAPIKey, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := responsesToAnthropicForAccount(&apicompat.ResponsesRequest{Model: tc.model, Input: input}, &Account{Type: tc.typ, Extra: map[string]any{"disable_mid_conversation_system": tc.disabled}})
			require.NoError(t, err)
			require.Equal(t, tc.inline, out.Messages[len(out.Messages)-1].Role == "system")
		})
	}
}
