package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// The actual upstream account, not a client-facing alias, determines whether
// mid-conversation instructions are legal. Unknown and OAuth providers retain
// the legacy top-level hoisting behavior.
func responsesToAnthropicForAccount(req *apicompat.ResponsesRequest, account *Account) (*apicompat.AnthropicRequest, error) {
	if account == nil {
		return apicompat.ResponsesToAnthropicRequest(req)
	}
	disabled, _ := account.Extra["disable_mid_conversation_system"].(bool)
	return apicompat.ResponsesToAnthropicRequestWithOptions(req, apicompat.ResponsesAnthropicOptions{
		MidConversationSystem: account.Type == AccountTypeAPIKey && claude.SupportsMidConversationSystem(req.Model) && !disabled,
	})
}
