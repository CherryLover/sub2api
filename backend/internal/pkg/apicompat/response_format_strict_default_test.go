package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatResponseFormatJSONSchemaPreservesExplicitAndImplicitStrict(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		wantStrict  bool
	}{
		{"omitted defaults false", `{"type":"json_schema","json_schema":{"name":"out","schema":{"type":"object","properties":{"ok":{"type":"boolean"}}}}}`, false},
		{"explicit true", `{"type":"json_schema","json_schema":{"name":"out","strict":true,"schema":{"type":"object"}}}`, true},
		{"explicit false", `{"type":"json_schema","json_schema":{"name":"out","strict":false,"schema":{"type":"object"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &ChatCompletionsRequest{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}}, ResponseFormat: json.RawMessage(tc.input)}
			resp, err := ChatCompletionsToResponses(req)
			require.NoError(t, err)
			require.NotNil(t, resp.Text)
			var format map[string]any
			require.NoError(t, json.Unmarshal(resp.Text.Format, &format))
			require.Equal(t, tc.wantStrict, format["strict"])
			require.Equal(t, "json_schema", format["type"])
		})
	}
}
