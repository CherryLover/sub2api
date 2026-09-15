package service

import (
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Only direct input-item metadata belongs to the Codex transport. Never strip
// same-named user content, tool arguments, or top-level application fields.
func stripOpenAIOAuthInputMetadataFields(req map[string]any) bool {
	changed := false
	input, _ := req["input"].([]any)
	for _, value := range input {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := item["internal_chat_message_metadata_passthrough"]; exists {
			delete(item, "internal_chat_message_metadata_passthrough")
			changed = true
		}
	}
	return changed
}
func stripOpenAIOAuthInputMetadataBody(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	normalized := body
	changed := false
	for i, item := range input.Array() {
		if !item.IsObject() || !item.Get("internal_chat_message_metadata_passthrough").Exists() {
			continue
		}
		next, err := sjson.DeleteBytes(normalized, fmt.Sprintf("input.%d.internal_chat_message_metadata_passthrough", i))
		if err != nil {
			return body, false, fmt.Errorf("normalize oauth input metadata: %w", err)
		}
		normalized = next
		changed = true
	}
	return normalized, changed, nil
}
func normalizeOpenAIOAuthInputMetadataForAccount(body []byte, account *Account) ([]byte, bool, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return body, false, nil
	}
	return stripOpenAIOAuthInputMetadataBody(body)
}
