package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const gemini403RequestScopeContextKey = "gemini.request_scoped_403"
const geminiRequest403ClientMessage = "The upstream denied access to a resource required by this request. Check resource permissions before retrying."

// A generic PERMISSION_DENIED is not proof of an invalid credential. One other
// account may serve a self-contained request, but a second denial must not
// poison the pool. No global account state is inferred from these observations.
// In particular, another account's 200 is not evidence that the first identity
// is invalid: resource permissions may legitimately differ between identities.
func (s *GeminiMessagesCompatService) handleGeminiRequest403(
	ctx context.Context, c *gin.Context, account *Account, resp *http.Response,
	responseBody, requestBody []byte, protocol string,
) (bool, error) {
	if resp == nil || resp.StatusCode != http.StatusForbidden || account == nil || c == nil {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	body := unwrapIfNeeded(account.Type == AccountTypeOAuth, responseBody)
	if gemini403IsAccountFailure(resp.Header, body) {
		return false, nil // Existing explicit credential/project error policy stays authoritative.
	}
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	requestID := resp.Header.Get("x-request-id")
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: http.StatusForbidden, UpstreamRequestID: requestID,
		Kind: "request_error", Message: message,
	})
	// A bound upstream resource and an explicit content/policy denial must never
	// be replayed using a different identity. Native cache ownership checks run
	// even earlier and keep their existing response contract.
	if geminiCachedContentBoundFromContext(ctx) || gemini403HasBoundResource(parseRawJSONView(requestBody), 0) || gemini403IsPolicyDenial(body) {
		return true, writeGeminiRequest403(c, protocol, http.StatusForbidden)
	}
	if _, attempted := c.Get(gemini403RequestScopeContextKey); attempted {
		return true, writeGeminiRequest403(c, protocol, http.StatusBadRequest)
	}
	c.Set(gemini403RequestScopeContextKey, account.ID)
	return true, &UpstreamFailoverError{
		StatusCode:             http.StatusForbidden,
		ResponseBody:           []byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"Upstream request permission denied"}}`),
		ResponseHeaders:        resp.Header.Clone(),
		RequestScopedTransient: true,
	}
}

func gemini403IsAccountFailure(headers http.Header, body []byte) bool {
	if isGeminiInsufficientScope(headers, body) {
		return true
	}
	for _, detail := range gjson.GetBytes(body, "error.details").Array() {
		switch detail.Get("reason").String() {
		case "ACCESS_TOKEN_SCOPE_INSUFFICIENT", "API_KEY_INVALID", "API_KEY_EXPIRED",
			"API_KEY_SERVICE_BLOCKED", "API_KEY_HTTP_REFERRER_BLOCKED", "API_KEY_IP_ADDRESS_BLOCKED",
			"API_KEY_ANDROID_APP_BLOCKED", "API_KEY_IOS_APP_BLOCKED", "SERVICE_DISABLED", "BILLING_DISABLED", "CONSUMER_INVALID":
			return true
		}
	}
	return false
}

func gemini403IsPolicyDenial(body []byte) bool {
	for _, detail := range gjson.GetBytes(body, "error.details").Array() {
		switch detail.Get("reason").String() {
		case "SAFETY", "PROHIBITED_CONTENT", "CONTENT_POLICY_VIOLATION", "BLOCKLIST", "IMAGE_SAFETY":
			return true
		}
	}
	msg := strings.ToLower(extractUpstreamErrorMessage(body))
	return strings.Contains(msg, "safety policy") || strings.Contains(msg, "content policy") ||
		strings.Contains(msg, "prohibited content") || strings.Contains(msg, "blocked by policy") ||
		strings.Contains(msg, "cachedcontent not found")
}

func gemini403HasBoundResource(value gjson.Result, depth int) bool {
	if depth > 32 {
		return true
	} // Uninspectable input is not eligible for identity migration.
	if !value.IsObject() && !value.IsArray() {
		return false
	}
	found := false
	value.ForEach(func(key, item gjson.Result) bool {
		switch key.String() {
		case "cachedContent", "fileUri", "file_uri":
			if item.Exists() && item.String() != "" {
				found = true
				return false
			}
		}
		found = gemini403HasBoundResource(item, depth+1)
		return !found
	})
	return found
}

func writeGeminiRequest403(c *gin.Context, protocol string, status int) error {
	payload := gin.H{"error": gin.H{"type": "invalid_request_error", "message": geminiRequest403ClientMessage}}
	switch protocol {
	case "native":
		code := "PERMISSION_DENIED"
		if status == http.StatusBadRequest {
			code = "INVALID_ARGUMENT"
		}
		payload = gin.H{"error": gin.H{"code": status, "status": code, "message": geminiRequest403ClientMessage}}
	case "claude":
		payload["type"] = "error"
	}
	MarkResponseCommitted(c)
	if gatewaySSEStarted(c) {
		MarkOpsStreamError(c, "invalid_request_error", geminiRequest403ClientMessage, status)
		c.SSEvent("error", payload)
		c.Writer.Flush()
	} else {
		c.JSON(status, payload)
	}
	return errors.New("gemini request permission denied")
}
