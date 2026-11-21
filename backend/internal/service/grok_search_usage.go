package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

func grokSearchCountFromUsage(usage gjson.Result) (int, bool) {
	details := usage.Get("server_side_tool_usage_details")
	if !details.IsObject() {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	total := 0
	for _, key := range []string{"web_search_calls", "x_search_calls", "document_search_calls"} {
		value := details.Get(key)
		if !value.Exists() {
			continue
		}
		if value.Type != gjson.Number {
			return 0, false
		}
		count, err := strconv.ParseUint(value.Raw, 10, strconv.IntSize-1)
		if err != nil || count > uint64(maxInt-total) {
			return 0, false
		}
		total += int(count)
	}
	return total, true
}
func grokSearchTerminalUsage(data []byte) (int, bool) {
	switch strings.TrimSpace(gjson.GetBytes(data, "type").String()) {
	case "response.completed", "response.incomplete", "response.done":
		return grokSearchCountFromUsage(gjson.GetBytes(data, "response.usage"))
	}
	return 0, false
}

// Once final usage is observed, late or duplicate item events cannot add a
// second charge. The typed counter belongs to one request, never an account.
type grokSearchStreamCounter struct {
	total         int
	authoritative bool
	seen          map[string]struct{}
}

func (c *grokSearchStreamCounter) observe(data []byte) int {
	if count, ok := grokSearchTerminalUsage(data); ok {
		c.total = count
		c.authoritative = true
		return count
	}
	if c.authoritative {
		return c.total
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	c.total += countGrokNativeSearchCallsInSSEDataDedup(data, c.seen)
	return c.total
}
