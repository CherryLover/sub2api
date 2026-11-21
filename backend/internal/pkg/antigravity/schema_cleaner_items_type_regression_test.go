package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanJSONSchemaItemsOnlyAllowedForArrays(t *testing.T) {
	const input = `{"type":"object","properties":{"uri":{"type":["string","array"],"items":{"type":"string"}},"paths":{"type":["array","string"],"items":{"type":"integer"}},"fallback":{"type":"array","items":17},"inferred":{"items":{"type":"integer"}},"nested":{"type":"array","items":{"type":"object","properties":{"uri":{"type":["string","array"],"items":{"type":"string"}}}}}}}`
	const expected = `{"type":"object","properties":{"uri":{"type":"string"},"paths":{"type":"array","items":{"type":"integer"}},"fallback":{"type":"array","items":{"type":"string"}},"inferred":{"type":"array","items":{"type":"integer"}},"nested":{"type":"array","items":{"type":"object","properties":{"uri":{"type":"string"}}}}}}`
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(input), &schema))
	got := CleanJSONSchema(schema)
	actual, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, expected, string(actual))
}
