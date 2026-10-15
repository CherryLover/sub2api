package apicompat

import "fmt"

// placeAnthropicSystemMessages leaves previous turns byte-stable for prompt
// caching and attaches a later instruction only after a user/tool-result turn.
// Call after normalizing tool pairs so an instruction cannot split a call/result.
func placeAnthropicSystemMessages(messages []AnthropicMessage) ([]AnthropicMessage, error) {
	out := make([]AnthropicMessage, 0, len(messages))
	var pending []AnthropicContentBlock
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if len(out) == 0 || out[len(out)-1].Role != "user" {
			return fmt.Errorf("mid-conversation system instruction requires user input or tool results before the next assistant turn; disable_mid_conversation_system can restore legacy hoisting for this provider")
		}
		out = append(out, anthropicMessageFromBlocks("system", pending))
		pending = nil
		return nil
	}
	for _, message := range messages {
		switch message.Role {
		case "system":
			pending = append(pending, parseContentBlocks(message.Content)...)
		case "assistant":
			if err := flush(); err != nil {
				return nil, err
			}
			out = append(out, message)
		default:
			out = append(out, message)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}
