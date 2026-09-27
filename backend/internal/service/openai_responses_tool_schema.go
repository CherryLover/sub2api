package service

import (
	"bytes"
	"sort"

	"github.com/tidwall/gjson"
)

const (
	openAIResponsesToolSchemaMaxDepth     = 12
	openAIResponsesToolSchemaFallbackType = `"object"`
	openAIResponsesToolSchemaNullLiteral  = "null"
)

type openAIResponsesToolSchemaEdit struct {
	offset      int
	length      int
	replacement string
}

// sanitizeOpenAIResponsesToolParameterTypes repairs invalid JSON-Schema
// members inside tool schema roots. It deliberately does not descend into
// instance-data keywords such as default/examples/const/enum.
//
// Edits are collected as byte spans and the request is rewritten once, keeping
// the lite branch's bounded-allocation behavior for large Responses bodies.
func sanitizeOpenAIResponsesToolParameterTypes(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}

	edits := make([]openAIResponsesToolSchemaEdit, 0, 4)
	collectOpenAIResponsesToolSchemaEdits(body, gjson.GetBytes(body, "tools"), 0, &edits)
	if input := gjson.GetBytes(body, "input"); input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			if item.IsObject() {
				collectOpenAIResponsesToolSchemaEdits(body, item.Get("tools"), 0, &edits)
			}
			return true
		})
	}
	if len(edits) == 0 {
		return body, false, nil
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].offset < edits[j].offset })
	out := make([]byte, 0, len(body)+len(edits)*len(openAIResponsesToolSchemaFallbackType))
	cursor := 0
	for _, edit := range edits {
		if edit.offset < cursor || edit.offset < 0 || edit.offset+edit.length > len(body) {
			continue
		}
		out = append(out, body[cursor:edit.offset]...)
		out = append(out, edit.replacement...)
		cursor = edit.offset + edit.length
	}
	out = append(out, body[cursor:]...)
	return out, true, nil
}

func collectOpenAIResponsesToolSchemaEdits(
	body []byte, tools gjson.Result, depth int, edits *[]openAIResponsesToolSchemaEdit,
) {
	if depth > openAIResponsesToolSchemaMaxDepth || !tools.IsArray() {
		return
	}
	tools.ForEach(func(_, tool gjson.Result) bool {
		if !tool.IsObject() {
			return true
		}
		for _, suffix := range []string{"parameters", "function.parameters", "input_schema"} {
			schema := tool.Get(suffix)
			if schema.IsObject() {
				collectOpenAIResponsesJSONSchemaEdits(body, schema, 0, edits)
			}
		}
		collectOpenAIResponsesToolSchemaEdits(body, tool.Get("tools"), depth+1, edits)
		return true
	})
}

func collectOpenAIResponsesJSONSchemaEdits(
	body []byte, schema gjson.Result, depth int, edits *[]openAIResponsesToolSchemaEdit,
) {
	if depth > openAIResponsesToolSchemaMaxDepth || !schema.IsObject() {
		return
	}

	if typ := schema.Get("type"); typ.Type == gjson.Null && typ.Raw == openAIResponsesToolSchemaNullLiteral {
		appendOpenAIResponsesToolSchemaReplacement(body, typ, openAIResponsesToolSchemaFallbackType, edits)
	}
	if required := schema.Get("required"); required.Type == gjson.Null && required.Raw == openAIResponsesToolSchemaNullLiteral {
		if offset, length, ok := openAIResponsesJSONObjectMemberSpan(body, required, "required"); ok {
			*edits = append(*edits, openAIResponsesToolSchemaEdit{offset: offset, length: length})
		}
	}

	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
		obj := schema.Get(key)
		if !obj.IsObject() {
			continue
		}
		obj.ForEach(func(_, child gjson.Result) bool {
			if child.IsObject() {
				collectOpenAIResponsesJSONSchemaEdits(body, child, depth+1, edits)
			}
			return true
		})
	}
	for _, key := range []string{
		"additionalProperties", "additionalItems", "contains", "not", "if", "then", "else",
		"propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema",
	} {
		child := schema.Get(key)
		if child.IsObject() {
			collectOpenAIResponsesJSONSchemaEdits(body, child, depth+1, edits)
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		arr := schema.Get(key)
		if !arr.IsArray() {
			continue
		}
		arr.ForEach(func(_, child gjson.Result) bool {
			if child.IsObject() {
				collectOpenAIResponsesJSONSchemaEdits(body, child, depth+1, edits)
			}
			return true
		})
	}
	items := schema.Get("items")
	switch {
	case items.IsObject():
		collectOpenAIResponsesJSONSchemaEdits(body, items, depth+1, edits)
	case items.IsArray():
		items.ForEach(func(_, child gjson.Result) bool {
			if child.IsObject() {
				collectOpenAIResponsesJSONSchemaEdits(body, child, depth+1, edits)
			}
			return true
		})
	}
}

func appendOpenAIResponsesToolSchemaReplacement(
	body []byte, value gjson.Result, replacement string, edits *[]openAIResponsesToolSchemaEdit,
) {
	end := value.Index + len(value.Raw)
	if value.Index <= 0 || end > len(body) || !bytes.Equal(body[value.Index:end], []byte(value.Raw)) {
		return
	}
	*edits = append(*edits, openAIResponsesToolSchemaEdit{
		offset: value.Index, length: len(value.Raw), replacement: replacement,
	})
}

// openAIResponsesJSONObjectMemberSpan returns the byte range for a fixed schema
// member and consumes one adjacent comma, so deleting the range keeps valid JSON.
func openAIResponsesJSONObjectMemberSpan(body []byte, value gjson.Result, key string) (int, int, bool) {
	if value.Index <= 0 || value.Index+len(value.Raw) > len(body) {
		return 0, 0, false
	}
	i := value.Index - 1
	for i >= 0 && isOpenAIResponsesJSONSpace(body[i]) {
		i--
	}
	if i < 0 || body[i] != ':' {
		return 0, 0, false
	}
	i--
	for i >= 0 && isOpenAIResponsesJSONSpace(body[i]) {
		i--
	}
	keyToken := []byte(`"` + key + `"`)
	keyEnd := i + 1
	keyStart := keyEnd - len(keyToken)
	if keyStart < 0 || !bytes.Equal(body[keyStart:keyEnd], keyToken) {
		return 0, 0, false
	}

	start := keyStart
	end := value.Index + len(value.Raw)
	for end < len(body) && isOpenAIResponsesJSONSpace(body[end]) {
		end++
	}

	j := start - 1
	for j >= 0 && isOpenAIResponsesJSONSpace(body[j]) {
		j--
	}
	if j >= 0 && body[j] == ',' {
		start = j
		return start, end - start, true
	}
	if end < len(body) && body[end] == ',' {
		end++
		for end < len(body) && isOpenAIResponsesJSONSpace(body[end]) {
			end++
		}
	}
	return start, end - start, true
}

func isOpenAIResponsesJSONSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
