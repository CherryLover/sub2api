package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/tidwall/gjson"
)

func supportsNativeAnthropicToolSearch(account *Account, model string) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return false
	}
	var metadata []byte
	switch openai.GPT62ModelID(model) {
	case "gpt-6.2-sol":
		metadata = openai.CodexGPT62SolMetadata
	case "gpt-6.2-astra":
		metadata = openai.CodexGPT62AstraMetadata
	default:
		if openai.IsGPT61TerraModelSpelling(model) {
			metadata = openai.CodexGPT61TerraMetadata
		}
	}
	return len(metadata) > 0 && gjson.GetBytes(metadata, "supports_search_tool").Bool()
}
