package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeGrokDelegatedAgentMessages turns Codex delegated agent_message
// envelopes into user messages while keeping the original item order.
// Grok has no agent_message input type; dropping it loses the delegated task.
func normalizeGrokDelegatedAgentMessages(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, nil
	}
	items := input.Array()
	var result []json.RawMessage
	changed := false
	for _, item := range items {
		if item.Get("type").String() != "agent_message" {
			result = append(result, json.RawMessage(item.Raw))
			continue
		}
		changed = true
		content := grokDelegatedAgentMessageText(item.Get("content"))
		if strings.TrimSpace(content) == "" {
			continue
		}
		message, err := json.Marshal(map[string]any{
			"type":    "message",
			"role":    "user",
			"content": content,
		})
		if err != nil {
			return nil, fmt.Errorf("serialize delegated agent message: %w", err)
		}
		result = append(result, message)
	}
	if !changed {
		return body, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode delegated agent messages: %w", err)
	}
	return sjson.SetRawBytes(body, "input", encoded)
}

// These parts are already plaintext in the client-side envelope; do not try
// to decrypt encrypted_content or accept unsupported object shapes as text.
func grokDelegatedAgentMessageText(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.String()
	}
	if !content.IsArray() {
		return ""
	}
	var text strings.Builder
	for _, part := range content.Array() {
		switch part.Get("type").String() {
		case "text", "input_text":
			_, _ = text.WriteString(part.Get("text").String())
		case "encrypted_content":
			_, _ = text.WriteString(part.Get("encrypted_content").String())
		}
	}
	return text.String()
}
