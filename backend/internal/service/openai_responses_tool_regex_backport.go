package service

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// sanitizeOpenAIResponsesToolSchemaPatterns only alters schema constraints in
// Responses tool declarations. Large request bodies and instance-valued fields
// (default, examples, const, enum) retain their original JSON bytes. The lite
// branch uses targeted byte-range edits rather than re-marshalling the request.
func sanitizeOpenAIResponsesToolSchemaPatterns(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !bytes.Contains(body, []byte(`"pattern`)) {
		return body, false, nil
	}
	const maxPasses = openAIResponsesToolSchemaMaxDepth + 1
	changed := false
	current := body
	for pass := 0; pass < maxPasses; pass++ {
		edits := make([]openAIResponsesToolSchemaEdit, 0, 4)
		collectOpenAIResponsesRegexToolEdits(current, gjson.GetBytes(current, "tools"), 0, &edits)
		if input := gjson.GetBytes(current, "input"); input.IsArray() {
			input.ForEach(func(_, item gjson.Result) bool {
				if item.IsObject() {
					collectOpenAIResponsesRegexToolEdits(current, item.Get("tools"), 0, &edits)
				}
				return true
			})
		}
		if len(edits) == 0 {
			return current, changed, nil
		}
		sort.Slice(edits, func(i, j int) bool { return edits[i].offset < edits[j].offset })
		var out []byte
		cursor := 0
		for _, edit := range edits {
			if edit.offset < cursor || edit.offset < 0 || edit.offset+edit.length > len(current) {
				continue
			}
			if out == nil {
				out = make([]byte, 0, len(current)+len(edits)*4)
			}
			out = append(out, current[cursor:edit.offset]...)
			out = append(out, edit.replacement...)
			cursor = edit.offset + edit.length
		}
		if out == nil {
			return current, changed, nil
		}
		out = append(out, current[cursor:]...)
		current, changed = out, true
	}
	return current, changed, nil
}

func collectOpenAIResponsesRegexToolEdits(body []byte, tools gjson.Result, depth int, edits *[]openAIResponsesToolSchemaEdit) {
	if depth > openAIResponsesToolSchemaMaxDepth || !tools.IsArray() {
		return
	}
	tools.ForEach(func(_, tool gjson.Result) bool {
		if !tool.IsObject() {
			return true
		}
		for _, suffix := range []string{"parameters", "function.parameters", "input_schema"} {
			if schema := tool.Get(suffix); schema.IsObject() {
				collectOpenAIResponsesRegexSchemaEdits(body, schema, 0, edits)
			}
		}
		collectOpenAIResponsesRegexToolEdits(body, tool.Get("tools"), depth+1, edits)
		return true
	})
}

func collectOpenAIResponsesRegexSchemaEdits(body []byte, schema gjson.Result, depth int, edits *[]openAIResponsesToolSchemaEdit) {
	if depth > openAIResponsesToolSchemaMaxDepth || !schema.IsObject() {
		return
	}
	if pattern := schema.Get("pattern"); pattern.Type == gjson.String && openAIRegexUnsupported(pattern.Str) {
		if offset, length, ok := openAIResponsesJSONObjectMemberSpan(body, pattern, "pattern"); ok {
			*edits = append(*edits, openAIResponsesToolSchemaEdit{offset: offset, length: length})
		}
	}
	// Each map value is RawMessage, preserving large literals and unrelated
	// valid subschemas when an unsupported pattern key is removed.
	patternMap := schema.Get("patternProperties")
	droppedMap := false
	if patternMap.IsObject() {
		var entries map[string]json.RawMessage
		if err := json.Unmarshal([]byte(patternMap.Raw), &entries); err == nil {
			for key := range entries {
				if openAIRegexUnsupported(key) {
					delete(entries, key)
					droppedMap = true
				}
			}
			if droppedMap {
				if replacement, err := json.Marshal(entries); err == nil {
					appendOpenAIResponsesToolSchemaReplacement(body, patternMap, string(replacement), edits)
				}
				// Deleting a pattern key makes previously matching arguments
				// additional properties. Do not strand them behind false.
				if additional := schema.Get("additionalProperties"); additional.Exists() && additional.Raw != "true" {
					appendOpenAIResponsesToolSchemaReplacement(body, additional, "true", edits)
				}
			}
		}
	}
	for _, key := range []string{"properties", "$defs", "definitions", "dependentSchemas"} {
		obj := schema.Get(key)
		if !obj.IsObject() {
			continue
		}
		obj.ForEach(func(_, child gjson.Result) bool {
			if child.IsObject() {
				collectOpenAIResponsesRegexSchemaEdits(body, child, depth+1, edits)
			}
			return true
		})
	}
	// After a patternProperties map replacement, defer nested sanitization
	// until the following pass to avoid overlapping edits.
	if patternMap.IsObject() && !droppedMap {
		patternMap.ForEach(func(_, child gjson.Result) bool {
			if child.IsObject() {
				collectOpenAIResponsesRegexSchemaEdits(body, child, depth+1, edits)
			}
			return true
		})
	}
	for _, key := range []string{"additionalProperties", "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema"} {
		if droppedMap && key == "additionalProperties" {
			continue
		}
		child := schema.Get(key)
		if child.IsObject() {
			collectOpenAIResponsesRegexSchemaEdits(body, child, depth+1, edits)
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems", "items"} {
		child := schema.Get(key)
		if child.IsObject() {
			collectOpenAIResponsesRegexSchemaEdits(body, child, depth+1, edits)
		}
		if child.IsArray() {
			child.ForEach(func(_, item gjson.Result) bool {
				if item.IsObject() {
					collectOpenAIResponsesRegexSchemaEdits(body, item, depth+1, edits)
				}
				return true
			})
		}
	}
}

// openAIRegexUnsupported approximates the Python re restrictions enforced for
// OpenAI tool schema patterns. Unlike general Go regexp syntax, escaped
// property classes (\\p) and non-Python named groups must be stripped.
func openAIRegexUnsupported(pattern string) bool {
	inClass := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				return true
			}
			ch := pattern[i+1]
			if openAIRegexIsASCII(ch) && !strings.ContainsRune("abfnrtvdDsSwWAZBNuUx", rune(ch)) {
				return true
			}
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if !inClass && i+2 < len(pattern) && pattern[i+1] == '?' && strings.ContainsRune("=!<'", rune(pattern[i+2])) {
				return true
			}
		}
	}
	return false
}

func openAIRegexIsASCII(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' }
