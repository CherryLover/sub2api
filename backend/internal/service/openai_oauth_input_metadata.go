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

// Apply edits to each input item and rebuild the surrounding body once.
// Preserve the old decoder path for malformed or non-addressable JSON views.
func stripOpenAIOAuthInputMetadataBody(body []byte) ([]byte, bool, error) {
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return body, false, nil
	}
	start, end := input.Index, input.Index+len(input.Raw)
	if !gjson.ValidBytes(body) || start < 0 || end > len(body) || string(body[start:end]) != input.Raw {
		return stripOpenAIOAuthInputMetadataBodyLegacy(body)
	}
	const field = "internal_chat_message_metadata_passthrough"
	var items []string
	changed := false
	var editErr error
	input.ForEach(func(_, item gjson.Result) bool {
		raw := item.Raw
		if item.IsObject() && item.Get(field).Exists() {
			raw, editErr = sjson.Delete(raw, field)
			if editErr != nil {
				return false
			}
			changed = true
		}
		items = append(items, raw)
		return true
	})
	if editErr != nil {
		return body, false, fmt.Errorf("normalize oauth input metadata: %w", editErr)
	}
	if !changed {
		return body, false, nil
	}
	out := make([]byte, 0, len(body))
	out = append(out, body[:start]...)
	out = append(out, '[')
	for i, item := range items {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, item...)
	}
	out = append(out, ']')
	out = append(out, body[end:]...)
	return out, true, nil
}

func stripOpenAIOAuthInputMetadataBodyLegacy(body []byte) ([]byte, bool, error) {
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
