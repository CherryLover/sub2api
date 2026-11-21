package service

import (
	"bytes"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Message-level controls follow the final outgoing beta header. Do not add
// betas implicitly or alter top-level effort and nonempty conversation turns.
func stripAnthropicMessageOutputConfigUnlessBeta(body []byte, anthropicBetaHeader string) ([]byte, bool) {
	if anthropicBetaTokensContains(anthropicBetaHeader, claude.BetaMidConversationOutputConfig) ||
		anthropicBetaTokensContains(anthropicBetaHeader, claude.BetaPerTurnControl) {
		return body, false
	}
	// 快速路径：body 中不含 output_config 字面量时无需解析。
	if !bytes.Contains(body, []byte("output_config")) && !bytes.Contains(body, []byte(`\u`)) {
		return body, false
	}
	msgsRes := gjson.GetBytes(body, "messages")
	if !msgsRes.Exists() || !msgsRes.IsArray() {
		return body, false
	}

	hasMessageOutputConfig := false
	for _, msg := range msgsRes.Array() {
		if msg.Get("output_config").Exists() {
			hasMessageOutputConfig = true
			break
		}
	}
	if !hasMessageOutputConfig {
		return body, false
	}

	var messages []json.RawMessage
	if err := json.Unmarshal([]byte(msgsRes.Raw), &messages); err != nil {
		// gjson 的 IsArray 只做形态判断、不保证 JSON 完整合法；此分支保守返回原 body。
		return body, false
	}

	changed := false
	rebuilt := make([]json.RawMessage, 0, len(messages))
	for _, msg := range messages {
		if !gjson.GetBytes(msg, "output_config").Exists() {
			rebuilt = append(rebuilt, msg)
			continue
		}
		changed = true

		// 空正文的 system 控制消息整条删除；其余消息只剥字段。
		if gjson.GetBytes(msg, "role").String() == "system" &&
			!anthropicMessageContentHasBody(gjson.GetBytes(msg, "content")) {
			continue
		}
		stripped, err := sjson.DeleteBytes(msg, "output_config")
		if err != nil {
			// 不应发生：字段存在且 msg 是合法 JSON。保守整条保留，不产出半成品。
			rebuilt = append(rebuilt, msg)
			continue
		}
		rebuilt = append(rebuilt, json.RawMessage(stripped))
	}
	if !changed {
		return body, false
	}

	rebuiltBytes, err := json.Marshal(rebuilt)
	if err != nil {
		return body, false
	}
	out, err := sjson.SetRawBytes(body, "messages", rebuiltBytes)
	if err != nil {
		return body, false
	}
	return out, true
}

func anthropicMessageContentHasBody(content gjson.Result) bool {
	switch {
	case !content.Exists():
		return false
	case content.Type == gjson.String:
		return content.String() != ""
	case content.IsArray():
		var blocks []any
		if err := json.Unmarshal([]byte(content.Raw), &blocks); err != nil {
			return true // 无法解析时保守视为有正文
		}
		cleaned, _ := stripEmptyTextBlocksFromSlice(blocks)
		return len(cleaned) > 0
	default:
		// null 视为无正文；object / number / bool 等非标准形态保守保留。
		return content.Type != gjson.Null
	}
}
