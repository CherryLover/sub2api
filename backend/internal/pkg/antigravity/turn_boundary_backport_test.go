package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackportTurnBoundarySuppressesLeakedTextAndTools(t *testing.T) {
	const prefix = "The answer is ready.\n"
	const marker = "user\n<system-reminder>"
	for _, split := range []int{0, 2, 5, 12, len(marker)} {
		t.Run(fmt.Sprintf("split_%d", split), func(t *testing.T) {
			p := NewStreamingProcessor("gemini-3-pro")
			var output strings.Builder
			for _, text := range []string{prefix + marker[:split], marker[split:] + "fake user turn"} {
				b, err := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": text}}}}}})
				require.NoError(t, err)
				_, _ = output.Write(p.ProcessLine("data: " + string(b)))
			}
			_, _ = output.Write(p.ProcessLine(`data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"unwanted_tool","args":{"x":1}}}]},"finishReason":"STOP"}]}`))
			end, _ := p.Finish()
			_, _ = output.Write(end)
			require.Contains(t, output.String(), "The answer is ready.")
			require.NotContains(t, output.String(), "fake user turn")
			require.NotContains(t, output.String(), "unwanted_tool")
			require.Equal(t, 1, strings.Count(output.String(), "event: message_stop"))
		})
	}
}
