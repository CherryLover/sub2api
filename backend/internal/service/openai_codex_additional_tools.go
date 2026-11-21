package service

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Only UUID-shaped at_ declarations have a transport normalization; all other
// provider IDs remain opaque, and a reference is changed only if declared here.
func normalizeCodexAdditionalToolsID(id string) string {
	if len(id) != 39 || !strings.HasPrefix(id, "at_") || id[11] != '-' || id[16] != '-' || id[21] != '-' || id[26] != '-' {
		return id
	}
	compact := strings.ReplaceAll(id[3:], "-", "")
	if len(compact) != 32 {
		return id
	}
	if _, err := hex.DecodeString(compact); err != nil {
		return id
	}
	return "at_" + compact
}

func codexAdditionalToolsIDMappings(input []any) map[string]string {
	mappings := make(map[string]string)
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || item["type"] != "additional_tools" {
			continue
		}
		if id, ok := item["id"].(string); ok {
			if normalized := normalizeCodexAdditionalToolsID(id); normalized != id {
				mappings[id] = normalized
			}
		}
	}
	return mappings
}

func normalizeCodexAdditionalToolsIDsBody(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	items := input.Array()
	mappings := make(map[string]string)
	for _, item := range items {
		if id := item.Get("id"); item.Get("type").String() == "additional_tools" && id.Type == gjson.String {
			if normalized := normalizeCodexAdditionalToolsID(id.Str); normalized != id.Str {
				mappings[id.Str] = normalized
			}
		}
	}
	if len(mappings) == 0 {
		return body, false, nil
	}
	normalized := body
	changed := false
	for i, item := range items {
		typ := item.Get("type").String()
		if typ != "additional_tools" && typ != "item_reference" {
			continue
		}
		id := item.Get("id")
		if id.Type != gjson.String {
			continue
		}
		value, exists := mappings[id.Str]
		if !exists {
			continue
		}
		next, err := sjson.SetBytes(normalized, fmt.Sprintf("input.%d.id", i), value)
		if err != nil {
			return body, false, fmt.Errorf("normalize additional_tools ID: %w", err)
		}
		normalized = next
		changed = true
	}
	return normalized, changed, nil
}

func normalizeOpenAIOAuthInputCompatibilityBody(body []byte) ([]byte, bool, error) {
	cleaned, changed, err := stripOpenAIOAuthInputMetadataBody(body)
	if err != nil {
		return body, false, err
	}
	normalized, idsChanged, err := normalizeCodexAdditionalToolsIDsBody(cleaned)
	if err != nil {
		return body, false, err
	}
	return normalized, changed || idsChanged, nil
}

func normalizeOpenAIOAuthInputCompatibilityForAccount(body []byte, account *Account) ([]byte, bool, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return body, false, nil
	}
	return normalizeOpenAIOAuthInputCompatibilityBody(body)
}
