package service

import (
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
)

// Only the resolved client projection survives failover; retaining the original
// error frame would pin arbitrary output/usage and bypass response-body budgets.
type openAIStreamClientRule struct {
	status         int
	message        string
	skipMonitoring bool
}

func resolveOpenAIStreamClientRule(c *gin.Context, account *Account, payload []byte, message string) *openAIStreamClientRule {
	if c == nil || account == nil || len(payload) == 0 {
		return nil
	}
	typ := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if typ != "error" && typ != "response.failed" {
		return nil
	}
	svc := getBoundErrorPassthroughService(c)
	if svc == nil {
		return nil
	}
	body := openAIStreamFailedEventPassthroughBody(payload, message)
	status := openAIStreamFailedEventSemanticStatus(payload, message)
	rule := svc.MatchRule(account.Platform, status, body)
	if rule == nil {
		return nil
	}
	if !rule.PassthroughCode && rule.ResponseCode != nil {
		status = *rule.ResponseCode
	}
	msg := ExtractUpstreamErrorMessage(body)
	if !rule.PassthroughBody && rule.CustomMessage != nil {
		msg = *rule.CustomMessage
	}
	if status < 100 || status > 599 {
		status = http.StatusBadGateway
	}
	return &openAIStreamClientRule{status: status, message: strings.Clone(truncateString(sanitizeUpstreamErrorMessage(msg), 4096)), skipMonitoring: rule.SkipMonitoring}
}

// ApplyOpenAIStreamFailedFrameRule applies a previously resolved stream rule
// only when an attempt is exhausted. HTTP-status failures have no stream rule.
func ApplyOpenAIStreamFailedFrameRule(c *gin.Context, err *UpstreamFailoverError) (int, string, string, bool) {
	if err == nil || err.streamClientRule == nil {
		return 0, "", "", false
	}
	rule := err.streamClientRule
	if c != nil && rule.skipMonitoring {
		c.Set(OpsSkipPassthroughKey, true)
	}
	return rule.status, "upstream_error", rule.message, true
}
