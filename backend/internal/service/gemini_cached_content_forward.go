package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleGeminiCachedContentForwardError keeps cache-resource errors out of
// account-wide cooldown and failover policies. It returns a committed, sanitized
// client error for these cases; every other error retains the original body and
// follows the normal gateway policy. Ownership is enforced before binding ctx.
func (s *GeminiMessagesCompatService) handleGeminiCachedContentForwardError(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, requestBody []byte) (bool, error) {
	if resp == nil || resp.StatusCode < http.StatusBadRequest {
		return false, nil
	}
	boundGateway := isGeminiBoundCacheUpstreamPassthrough(ctx, account, resp.StatusCode)
	ref, refErr := FindGeminiCachedContentReference(requestBody)
	if !boundGateway && (refErr != nil || ref == nil) {
		return false, nil
	}
	// Read and restore so non-cache authorization or validation failures keep
	// their original behavior. The bounded error reader never buffers a huge body.
	body := s.readUpstreamErrorBody(resp)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if isGeminiCachedContentNotFound(resp.StatusCode, body) {
		setOpsUpstreamError(c, resp.StatusCode, GeminiCachedContentNotFoundMessage, "")
		c.Data(http.StatusForbidden, "application/json", []byte(GeminiCachedContentNotFoundResponse))
		return true, fmt.Errorf("%w", ErrGeminiCachedContentNotFound)
	}
	if boundGateway {
		message := "The account holding this cached content is temporarily unavailable"
		setOpsUpstreamError(c, resp.StatusCode, message, "")
		return true, s.writeGoogleError(c, resp.StatusCode, message)
	}
	return false, nil
}
