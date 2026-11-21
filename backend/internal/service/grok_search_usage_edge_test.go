//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestGrokSearchUsageRejectsInvalidCounters(t *testing.T) {
	for _, bad := range []string{`-1`, `1.5`, `"3"`, `null`, `true`, `1e300`, `9223372036854775808`} {
		t.Run(bad, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"output":[%s],"usage":{"server_side_tool_usage_details":{"x_search_calls":%s}}}`, grokXSearchItemJSON, bad))
			require.Equal(t, 1, countGrokNativeSearchCallsFromJSONBytes(body), "invalid usage must not fabricate a successful zero count")
		})
	}
	body := []byte(fmt.Sprintf(`{"output":[%s],"usage":{"server_side_tool_usage_details":{"web_search_calls":9223372036854775807,"x_search_calls":1}}}`, grokXSearchItemJSON))
	require.Equal(t, 1, countGrokNativeSearchCallsFromJSONBytes(body))
}
func TestGrokSearchUsageCanonicalEnvelopeAndFailedItems(t *testing.T) {
	body := []byte(fmt.Sprintf(`{"response":{"output":[%s]},"usage":{"server_side_tool_usage_details":{"x_search_calls":999}}}`, grokXSearchItemJSON))
	require.Equal(t, 1, countGrokNativeSearchCallsFromJSONBytes(body))
	require.Zero(t, countGrokNativeSearchCallsFromJSONBytes([]byte(`{"output":[`+grokFailedXSearchItemJSON+`]}`)))
}
func TestGrokSearchTerminalUsageCannotBeOverwrittenByLateItems(t *testing.T) {
	sse := withGrokUsage(grokCustomToolCallStream([]string{grokXSearchItemJSON}, `[]`), `{"x_search_calls":0}`)
	late := grokSSEFrame("response.output_item.done", `"output_index":9,"item":`+grokASearchItemJSON)
	require.True(t, json.Valid([]byte(strings.TrimSpace(strings.TrimPrefix(late, "data:")))))
	sse = strings.Replace(sse, "data: [DONE]", late+"data: [DONE]", 1)
	require.Zero(t, countGrokNativeSearchCallsFromSSEBody(sse))
	account := healthyGrokOAuthGatewayTestAccount(9242, "access-token")
	for _, path := range []string{"/v1/messages", "/v1/chat/completions"} {
		got := forwardGrokCustomToolCallStream(t, path, true, account, sse)
		require.Zero(t, got.searchCount)
	}
}
