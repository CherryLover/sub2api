package apicompat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ChatCompletionsToResponsesOptions controls upstream-specific replay only.
// Audio handling and the existing max-output floor are unchanged.
type ChatCompletionsToResponsesOptions struct{ ReplayAnthropicThinking bool }

func ChatCompletionsToResponsesWithOptions(req *ChatCompletionsRequest, opts ChatCompletionsToResponsesOptions) (*ResponsesRequest, error) {
	return chatCompletionsToResponses(req, false, opts.ReplayAnthropicThinking)
}

type ResponsesToChatCompletionsOptions struct{ AnthropicReasoningDetails bool }

// ChatReasoningDetail keeps signed content opaque to tool-name rewriting.
// The provider, not the gateway, verifies the enclosed signature on replay.
type ChatReasoningDetail struct {
	Type   string `json:"type"`
	Data   string `json:"data,omitempty"`
	Format string `json:"format,omitempty"`
	Index  int    `json:"index"`
}

const anthropicReasoningDetailFormat = "anthropic-claude-v1"

func decodeAnthropicThinking(encrypted string) (AnthropicContentBlock, error) {
	if !strings.HasPrefix(encrypted, anthropicThinkingEnvelopePrefix) {
		return AnthropicContentBlock{}, fmt.Errorf("invalid Anthropic thinking envelope prefix")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, anthropicThinkingEnvelopePrefix))
	if err != nil {
		return AnthropicContentBlock{}, fmt.Errorf("invalid Anthropic thinking envelope: %w", err)
	}
	var block AnthropicContentBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		return AnthropicContentBlock{}, fmt.Errorf("invalid Anthropic thinking block: %w", err)
	}
	if (block.Type != "thinking" || block.Signature == "") && (block.Type != "redacted_thinking" || block.Data == "") {
		return AnthropicContentBlock{}, fmt.Errorf("invalid Anthropic signed thinking block")
	}
	return block, nil
}
func anthropicReasoningDetailsToResponsesItems(raw json.RawMessage) []ResponsesInputItem {
	var entries []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	var kept []ChatReasoningDetail
	for _, entry := range entries {
		var d ChatReasoningDetail
		if json.Unmarshal(entry, &d) != nil || d.Format != anthropicReasoningDetailFormat || d.Type != "reasoning.encrypted" || d.Index < 0 {
			continue
		}
		if _, err := decodeAnthropicThinking(d.Data); err != nil {
			continue
		}
		kept = append(kept, d)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Index < kept[j].Index })
	var items []ResponsesInputItem
	for _, d := range kept {
		items = append(items, ResponsesInputItem{Type: "reasoning", EncryptedContent: d.Data})
	}
	return items
}
func anthropicThinkingReasoningDetail(encrypted string, index int) (ChatReasoningDetail, bool) {
	if _, err := decodeAnthropicThinking(encrypted); err != nil {
		return ChatReasoningDetail{}, false
	}
	return ChatReasoningDetail{Type: "reasoning.encrypted", Data: encrypted, Format: anthropicReasoningDetailFormat, Index: index}, true
}
func resToChatHandleOutputItemDone(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if !state.AnthropicReasoningDetails || state.Finalized || evt.Item == nil || evt.Item.Type != "reasoning" {
		return nil
	}
	if _, duplicate := state.emittedReasoningIndexes[evt.OutputIndex]; duplicate {
		return nil
	}
	detail, ok := anthropicThinkingReasoningDetail(evt.Item.EncryptedContent, state.NextReasoningIndex)
	if !ok {
		return nil
	}
	raw, err := json.Marshal([]ChatReasoningDetail{detail})
	if err != nil {
		return nil
	}
	if state.emittedReasoningIndexes == nil {
		state.emittedReasoningIndexes = make(map[int]struct{})
	}
	state.emittedReasoningIndexes[evt.OutputIndex] = struct{}{}
	state.NextReasoningIndex++
	return []ChatCompletionsChunk{makeChatDeltaChunk(state, ChatDelta{ReasoningDetails: raw})}
}
