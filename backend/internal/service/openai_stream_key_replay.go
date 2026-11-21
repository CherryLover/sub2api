package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/tidwall/gjson"
)

// OpenAIStreamKeyReplayAllowed reports whether an API-key Responses request can
// be replayed before any output reaches the client.
func OpenAIStreamKeyReplayAllowed(ctx context.Context, body []byte) bool {
	if ctx == nil || ctx.Err() != nil || !plainOpenAIStreamReplayTurn(body) || gjson.GetBytes(body, "prompt").Exists() {
		return false
	}
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		switch tool.Get("type").String() {
		case "function", "custom":
		default:
			return false
		}
	}
	return true
}

func plainOpenAIStreamReplayTurn(body []byte) bool {
	if len(body) > 16<<20 || !gjson.ValidBytes(body) || !openAIStreamReplayUniqueJSON(body) {
		return false
	}
	root := gjson.ParseBytes(body)
	if root.Get("previous_response_id").Exists() || root.Get("conversation").Exists() || (root.Get("background").Exists() && root.Get("background").Type != gjson.False) {
		return false
	}
	input := root.Get("input")
	if input.Type == gjson.String {
		return true
	}
	if !input.IsArray() {
		return false
	}
	for _, item := range input.Array() {
		typ, role := item.Get("type").String(), item.Get("role").String()
		if (typ != "" && typ != "message") || item.Get("id").Exists() || item.Get("encrypted_content").Exists() || item.Get("call_id").Exists() || item.Get("tool_calls").Exists() ||
			(role != "user" && role != "assistant" && role != "developer" && role != "system") {
			return false
		}
		content := item.Get("content")
		if content.Type == gjson.String {
			continue
		}
		if !content.IsArray() {
			return false
		}
		for _, part := range content.Array() {
			partType := part.Get("type").String()
			if (partType != "input_text" && partType != "output_text" && partType != "text") || part.Get("text").Type != gjson.String {
				return false
			}
		}
	}
	return true
}

// Duplicate members can bind differently upstream. Reject ambiguous or deeply
// nested requests for replay; never guess whether a hosted tool is safe.
func openAIStreamReplayUniqueJSON(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var consume func(int) bool
	consume = func(depth int) bool {
		if depth > 64 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return false
				}
				key, ok := keyToken.(string)
				if !ok {
					return false
				}
				if _, duplicate := seen[key]; duplicate {
					return false
				}
				seen[key] = struct{}{}
				if !consume(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for decoder.More() {
				if !consume(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !consume(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
