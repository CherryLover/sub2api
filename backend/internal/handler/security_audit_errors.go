package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) openAISecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *GatewayHandler) openAISecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *GatewayHandler) responsesSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	c.JSON(securityAuditStatus(decision), gin.H{"error": gin.H{
		"type": "api_error", "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *GatewayHandler) anthropicSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"type": "error", "error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func (h *OpenAIGatewayHandler) anthropicSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock {
		errType = "permission_error"
	}
	c.JSON(securityAuditStatus(decision), gin.H{"type": "error", "error": gin.H{
		"type": errType, "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision),
	}})
}

func googleSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	status := securityAuditStatus(decision)
	googleStatus := googleapi.HTTPStatusToGoogleStatus(status)
	if status == http.StatusServiceUnavailable {
		googleStatus = "UNAVAILABLE"
	}
	requestID := ""
	if c != nil && c.Request != nil {
		requestID = gatewayRequestID(c.Request.Context())
	}
	c.JSON(status, gin.H{"error": gin.H{
		"code": status, "message": securityAuditMessage(decision), "status": googleStatus,
		"details": []gin.H{{
			"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
			"reason": securityAuditErrorCode(decision), "domain": "sub2api.securityaudit",
			"metadata": gin.H{"request_id": requestID},
		}},
	}})
}

func writeSecurityAuditWSError(ctx context.Context, conn *coderws.Conn, decision *securityaudit.Decision) {
	if conn == nil || decision == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(gin.H{
		"event_id": "evt_prompt_guard_rejected", "type": "error",
		"error": gin.H{"type": "invalid_request_error", "code": securityAuditErrorCode(decision), "message": securityAuditMessage(decision)},
	})
	if err != nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
}

func securityAuditWSCloseStatus(decision *securityaudit.Decision) coderws.StatusCode {
	if decision == nil {
		return coderws.StatusInternalError
	}
	if decision.Kind == securityaudit.DecisionBlock {
		return coderws.StatusCode(4403)
	}
	return coderws.StatusTryAgainLater
}

func securityAuditWSCloseReason(decision *securityaudit.Decision) string {
	if decision == nil {
		return securityaudit.ErrorCodeUnavailable
	}
	code := securityAuditErrorCode(decision)
	if code == "" {
		return securityaudit.ErrorCodeUnavailable
	}
	return code
}
