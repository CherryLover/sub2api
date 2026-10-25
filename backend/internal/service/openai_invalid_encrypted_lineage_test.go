package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newEncryptedLineageTestContext(keyID int64, session string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: keyID})
	c.Request.Header.Set("Session_ID", session)
	return c
}

func TestOpenAIInvalidEncryptedLineageIsPerSessionAndDigest(t *testing.T) {
	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"gpt-6","input":[{"type":"reasoning","encrypted_content":"rejected","summary":[{"type":"summary_text","text":"keep"}]},{"type":"reasoning","encrypted_content":"fresh","summary":[{"type":"summary_text","text":"other"}]},{"type":"message","role":"user","content":"hello"}]}`)
	c := newEncryptedLineageTestContext(123, "conversation-A")
	key := svc.invalidEncryptedLineageKey(c, body)
	require.NotEmpty(t, key)
	svc.rememberOpenAIInvalidEncryptedLineage(key, []string{openAIInvalidEncryptedDigest("rejected")})
	out, changed := svc.stripKnownOpenAIInvalidEncrypted(c, body)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "input.0.encrypted_content").Exists())
	require.Equal(t, "keep", gjson.GetBytes(out, "input.0.summary.0.text").String())
	require.Equal(t, "fresh", gjson.GetBytes(out, "input.1.encrypted_content").String())
	require.Equal(t, "hello", gjson.GetBytes(out, "input.2.content").String())
	require.Equal(t, "rejected", gjson.GetBytes(body, "input.0.encrypted_content").String(), "original input is immutable")
	for _, scope := range []*gin.Context{
		newEncryptedLineageTestContext(124, "conversation-A"),
		newEncryptedLineageTestContext(123, "conversation-B"),
	} {
		passthrough, changed := svc.stripKnownOpenAIInvalidEncrypted(scope, body)
		require.False(t, changed)
		require.Equal(t, string(body), string(passthrough))
	}
}

func TestOpenAIInvalidEncryptedLineageDropsOnlyMatchingCompaction(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := newEncryptedLineageTestContext(456, "compaction-A")
	body := []byte(`{"input":[{"type":"compaction","encrypted_content":"old"},{"type":"compaction","encrypted_content":"new"},{"type":"message","content":"hi"}]}`)
	svc.rememberOpenAIInvalidEncryptedLineage(svc.invalidEncryptedLineageKey(c, body), collectOpenAIInvalidEncryptedDigests([]byte(`{"input":[{"type":"compaction","encrypted_content":"old"}]}`)))
	next, changed := svc.stripKnownOpenAIInvalidEncrypted(c, body)
	require.True(t, changed)
	require.Equal(t, "new", gjson.GetBytes(next, "input.0.encrypted_content").String())
	require.Equal(t, "message", gjson.GetBytes(next, "input.1.type").String())
}

func TestOpenAIInvalidEncryptedLineageUnauthenticatedDoesNotCache(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := []byte(`{"prompt_cache_key":"same","input":[{"type":"reasoning","encrypted_content":"secret"}]}`)
	require.Empty(t, svc.invalidEncryptedLineageKey(c, body))
	svc.rememberOpenAIInvalidEncryptedLineage("", []string{openAIInvalidEncryptedDigest("secret")})
	_, changed := svc.stripKnownOpenAIInvalidEncrypted(c, body)
	require.False(t, changed)
}
