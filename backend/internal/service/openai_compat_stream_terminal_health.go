package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// handleOpenAICompatStreamTerminalAccountSideEffects updates account health when
// a compatibility stream has already produced output and cannot safely replay
// the request on another account. Request-scoped overload and client errors
// must never disable an otherwise healthy account.
func (s *OpenAIGatewayService) handleOpenAICompatStreamTerminalAccountSideEffects(c *gin.Context, account *Account, payload []byte, message, upstreamModel string) {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || s.rateLimitService == nil {
		return
	}
	status := openAIStreamFailedEventSemanticStatus(payload, message)
	switch status {
	case http.StatusForbidden:
		// Only explicit credential revocation belongs to the account.
		code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
		switch code {
		case "invalid_api_key", "api_key_disabled", "invalid_token", "token_revoked", "invalid_credentials":
		default:
			return
		}
	case http.StatusUnauthorized:
		// Authentication failures are account-scoped.
	case http.StatusTooManyRequests:
		if isOpenAIRequestScopedCapacityShed(message, payload) {
			return
		}
	default:
		return
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	// Outer HTTP 200 headers do not establish a 429 reset; pass no stale
	// quota metadata into account health accounting.
	s.handleOpenAIAccountUpstreamError(ctx, account, status, nil, payload, upstreamModel)
}
