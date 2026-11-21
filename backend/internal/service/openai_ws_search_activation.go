package service

import (
	"encoding/json"

	"github.com/tidwall/gjson"
)

func openAIWSWebSearchDeclared(payload []byte) bool {
	fields := gjson.GetManyBytes(payload, "tools", "input")
	if gjsonToolsContainWebSearch(fields[0]) {
		return true
	}
	for _, item := range fields[1].Array() {
		if item.Get("type").String() == openAIAdditionalToolsItemType && gjsonToolsContainWebSearch(item.Get("tools")) {
			return true
		}
	}
	return false
}

// Called under writeMu, only after the first response.create was written.
func (c *openAIWSConn) recordInitialWebSearch(value any) {
	var body []byte
	switch payload := value.(type) {
	case json.RawMessage:
		body = payload
	case []byte:
		body = payload
	case map[string]any:
		if payload["type"] != "response.create" {
			return
		}
		c.responseCreateSent = true
		c.initialWebSearch = openAIToolsContainWebSearch(payload["tools"])
		input, _ := payload["input"].([]any)
		for _, raw := range input {
			if item, ok := raw.(map[string]any); ok && item["type"] == openAIAdditionalToolsItemType && openAIToolsContainWebSearch(item["tools"]) {
				c.initialWebSearch = true
			}
		}
		return
	default:
		return
	}
	if gjson.GetBytes(body, "type").String() == "response.create" {
		c.responseCreateSent = true
		c.initialWebSearch = openAIWSWebSearchDeclared(body)
	}
}

func (l *openAIWSConnLease) initializedWithoutWebSearch() bool {
	c, err := l.activeConn()
	if err != nil {
		return false
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.responseCreateSent && !c.initialWebSearch
}

// Search activation is optional. Keep its extra retained history independently
// bounded so it never grows the existing tool-recovery buffer without limit.
const openAIWSSearchReplayMaxBytes = 16 << 20

func boundedOpenAIWSSearchReplay(input, output []json.RawMessage, limit int) ([]json.RawMessage, bool) {
	if limit < 2 {
		return nil, false
	}
	remaining := limit - 2
	for _, items := range [][]json.RawMessage{input, output} {
		for _, item := range items {
			if len(item) >= remaining {
				return nil, false
			}
			remaining -= len(item) + 1
		}
	}
	result := make([]json.RawMessage, 0, len(input)+len(output))
	for _, items := range [][]json.RawMessage{input, output} {
		for _, item := range items {
			result = append(result, append(json.RawMessage(nil), item...))
		}
	}
	return result, true
}

type openAIWSSearchReplayState struct {
	items      []json.RawMessage
	complete   bool
	responseID string
}

func (s *openAIWSSearchReplayState) input(payload []byte, previous string) ([]json.RawMessage, bool, error) {
	if len(payload) > openAIWSSearchReplayMaxBytes || (previous != "" && (!s.complete || s.responseID != previous)) {
		return nil, false, nil
	}
	items, exists, err := buildOpenAIWSReplayInputSequence(s.items, s.complete, payload, previous != "")
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	bounded, ok := boundedOpenAIWSSearchReplay(items, nil, openAIWSSearchReplayMaxBytes)
	return bounded, ok, nil
}
func (s *openAIWSSearchReplayState) store(responseID string, input []json.RawMessage, inputComplete bool, output []json.RawMessage, outputComplete bool) {
	s.responseID = responseID
	s.items = nil
	s.complete = false
	if !inputComplete || !outputComplete {
		return
	}
	s.items, s.complete = boundedOpenAIWSSearchReplay(input, output, openAIWSSearchReplayMaxBytes)
}
