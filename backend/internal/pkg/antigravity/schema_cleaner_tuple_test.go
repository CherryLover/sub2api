package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanJSONSchemaDraft202012TuplePrefixItems(t *testing.T) {
	tests := []struct {
		name     string
		items    any
		hasItems bool
	}{
		{name: "no items"},
		{name: "closed tuple", items: false, hasItems: true},
		{name: "boolean true", items: true, hasItems: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prop := map[string]any{
				"type": "array",
				"prefixItems": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "object", "properties": map[string]any{"code": map[string]any{"type": "string"}}},
				},
			}
			if tt.hasItems {
				prop["items"] = tt.items
			}
			cleaned := CleanJSONSchema(map[string]any{"type": "object", "properties": map[string]any{"tuple": prop}})
			item := cleaned["properties"].(map[string]any)["tuple"].(map[string]any)
			require.NotContains(t, item, "prefixItems")
			child, ok := item["items"].(map[string]any)
			require.True(t, ok)
			require.NotEmpty(t, child["type"])
		})
	}
}

func TestCleanJSONSchemaTupleKeepsExplicitAdditionalItemsSchema(t *testing.T) {
	cleaned := CleanJSONSchema(map[string]any{
		"type":        "array",
		"prefixItems": []any{map[string]any{"type": "integer"}},
		"items":       map[string]any{"type": "boolean"},
	})
	require.NotContains(t, cleaned, "prefixItems")
	require.Equal(t, "boolean", cleaned["items"].(map[string]any)["type"])
}

func TestCleanJSONSchemaEmptyTupleDropsUnsupportedKeyword(t *testing.T) {
	cleaned := CleanJSONSchema(map[string]any{"type": "array", "prefixItems": []any{}, "items": map[string]any{"type": "string"}})
	require.NotContains(t, cleaned, "prefixItems")
	require.Equal(t, "string", cleaned["items"].(map[string]any)["type"])
}
