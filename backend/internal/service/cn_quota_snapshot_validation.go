package service

import (
	"github.com/tidwall/gjson"
	"strings"
)

// A valid sibling window must not hide a corrupt reported exhausted window.
// Validate the complete snapshot before absent-window cleanup or freshness updates.
func kimiQuotaSnapshotComplete(body []byte) bool {
	validCounts := func(node gjson.Result) bool {
		if !node.IsObject() {
			return false
		}
		limit, ok := cnParseF64(node.Get("limit").Value())
		if !ok || limit <= 0 {
			return false
		}
		remaining, ok := cnParseF64(node.Get("remaining").Value())
		return ok && remaining >= 0 && remaining <= limit
	}
	if limits := gjson.GetBytes(body, "limits"); limits.Exists() {
		if !limits.IsArray() {
			return false
		}
		for _, item := range limits.Array() {
			if !item.IsObject() {
				return false
			}
			if detail := item.Get("detail"); detail.Exists() && !validCounts(detail) {
				return false
			}
		}
	}
	if usage := gjson.GetBytes(body, "usage"); usage.Exists() && !validCounts(usage) {
		return false
	}
	if usages := gjson.GetBytes(body, "usages"); usages.Exists() {
		if !usages.IsObject() {
			return false
		}
		for _, key := range []string{"limit_5h", "limit_7d", "limit_month_total", "limit_month_code"} {
			if node := usages.Get(key); node.Exists() {
				if !node.IsObject() {
					return false
				}
				value, ok := cnParseF64(node.Get("used_ratio").Value())
				if !ok || value < 0 || value > 1 {
					return false
				}
			}
		}
	}
	return true
}

func zhipuQuotaSnapshotComplete(body []byte) bool {
	limits := gjson.GetBytes(body, "data.limits")
	if !limits.IsArray() {
		return false
	}
	for _, item := range limits.Array() {
		switch strings.ToUpper(strings.TrimSpace(item.Get("type").String())) {
		case "TOKENS_LIMIT", "CREDIT_LIMIT":
			percent, ok := cnParseF64(item.Get("percentage").Value())
			if !ok || percent < 0 || percent > 100 {
				return false
			}
		}
	}
	return true
}
