package antigravity

import "strings"

func isEmptyAbnormalGeminiTerminalResponse(geminiResp *GeminiResponse) bool {
	if geminiResp == nil || len(geminiResp.Candidates) == 0 {
		return false
	}
	cand := geminiResp.Candidates[0]
	switch strings.ToUpper(strings.TrimSpace(cand.FinishReason)) {
	case "MALFORMED_FUNCTION_CALL", "MALFORMED_RESPONSE", "UNEXPECTED_TOOL_CALL", "TOO_MANY_TOOL_CALLS", "OTHER":
	default:
		return false
	}
	if cand.Content != nil {
		for _, part := range cand.Content.Parts {
			if part.Text != "" || part.FunctionCall != nil || part.InlineData != nil {
				return false
			}
		}
	}
	return true
}

func isTerminalGeminiFinishReason(finishReason string) bool {
	upper := strings.ToUpper(strings.TrimSpace(finishReason))
	return upper != "" && upper != "FINISH_REASON_UNSPECIFIED"
}

func isTerminalGeminiBlockReason(blockReason string) bool {
	upper := strings.ToUpper(strings.TrimSpace(blockReason))
	return upper != "" && upper != "BLOCK_REASON_UNSPECIFIED" && upper != "BLOCKED_REASON_UNSPECIFIED"
}

func mapGeminiFinishReasonToClaudeStopReason(finishReason string, usedTool bool) string {
	if usedTool {
		return "tool_use"
	}
	switch strings.ToUpper(strings.TrimSpace(finishReason)) {
	case "MAX_TOKENS":
		return "max_tokens"
	case "SAFETY", "RECITATION", "LANGUAGE", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII",
		"IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION":
		return "refusal"
	default:
		return "end_turn"
	}
}
