package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var codexSelectedPageName = regexp.MustCompile(`^codex_apps_selected_page_part_([1-9][0-9]*)_of_([1-9][0-9]*)$`)

// Codex injects external context as standalone outputs, without a model tool call.
// Use the legacy user-message representation for HTTP Responses interoperability.
func normalizeCodexUntrustedContext(body []byte) ([]byte, bool) {
	if !mayContainCodexPageContextCandidate(body, "codex_apps_open_page", "codex_apps_selected_page_part_", "browser_info", "terminal_info", "goal_objective", "untrusted_context") || !codexPageContextHasUniqueJSONMembers(body) {
		return body, false
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(body, &request) != nil {
		return body, false
	}
	var input []json.RawMessage
	if json.Unmarshal(request["input"], &input) != nil {
		return body, false
	}
	changed := false
	for i, raw := range input {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		readString := func(key string) string {
			var value string
			_ = json.Unmarshal(item[key], &value)
			return value
		}
		if readString("type") != "function_call_output" || readString("namespace") != "agent" {
			continue
		}
		if callID, exists := item["call_id"]; exists {
			var value string
			if bytes.Equal(bytes.TrimSpace(callID), []byte("null")) || json.Unmarshal(callID, &value) != nil || strings.TrimSpace(value) != "" {
				continue
			}
		}
		name, output := readString("name"), readString("output")
		if !validCodexUntrustedContext(name, output) {
			continue
		}
		text := "<external_" + name + ">" + output + "</external_" + name + ">"
		if validCodexPageContext(name, output) {
			text = "<external_" + name + ">\n" + output + "\n</external_" + name + ">"
		}
		message := map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]string{{
				"type": "input_text",
				"text": text,
			}},
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return body, false
		}
		input[i] = encoded
		changed = true
	}
	if !changed {
		return body, false
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		return body, false
	}
	request["input"] = encodedInput
	encoded, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return encoded, true
}

func validCodexUntrustedContext(name, output string) bool {
	switch name {
	case "browser_info", "terminal_info", "goal_objective", "untrusted_context":
		return strings.TrimSpace(output) != ""
	default:
		return validCodexPageContext(name, output)
	}
}

func validCodexPageContext(name, output string) bool {
	if name == "codex_apps_open_page" {
		return validCodexPageSnapshot(output)
	}
	parts := codexSelectedPageName.FindStringSubmatch(name)
	if len(parts) != 3 {
		return false
	}
	part, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	total, err := strconv.Atoi(parts[2])
	if err != nil || part > total {
		return false
	}
	header, snapshot, ok := strings.Cut(output, "\n")
	if !ok || strings.TrimSuffix(header, "\r") != fmt.Sprintf("Companion Page snapshot part %d of %d:", part, total) || strings.TrimSpace(snapshot) == "" {
		return false
	}
	// A multi-part snapshot may split inside a JSON string; preserve each chunk verbatim.
	return total > 1 || validCodexPageSnapshot(snapshot)
}

func validCodexPageSnapshot(text string) bool {
	if !codexPageContextHasUniqueJSONMembers([]byte(text)) {
		return false
	}
	var snapshot map[string]json.RawMessage
	if json.Unmarshal([]byte(text), &snapshot) != nil {
		return false
	}
	id, exists := snapshot["page_id"]
	if !exists {
		return false
	}
	if bytes.Equal(bytes.TrimSpace(id), []byte("null")) {
		return true
	}
	var value string
	return json.Unmarshal(id, &value) == nil && strings.TrimSpace(value) != ""
}

// Avoid introducing upstream automation/delegation normalization into the lite handler.
// Escaped JSON strings are inspected by the decoder, not a byte-substring shortcut.
func mayContainCodexPageContextCandidate(body []byte, names ...string) bool {
	var request struct {
		Input []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"input"`
	}
	if json.Unmarshal(body, &request) != nil {
		return false
	}
	for _, item := range request.Input {
		if item.Type != "function_call_output" {
			continue
		}
		for _, name := range names {
			if item.Name == name || (strings.HasSuffix(name, "_") && strings.HasPrefix(item.Name, name)) {
				return true
			}
		}
	}
	return false
}

func codexPageContextHasUniqueJSONMembers(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if !consumeUniqueCodexPageContextJSON(decoder, 0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func consumeUniqueCodexPageContextJSON(decoder *json.Decoder, depth int) bool {
	if depth > 256 {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return true
	}
	switch delim {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return false
			}
			if _, exists := members[key]; exists {
				return false
			}
			members[key] = struct{}{}
			if !consumeUniqueCodexPageContextJSON(decoder, depth+1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !consumeUniqueCodexPageContextJSON(decoder, depth+1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}
