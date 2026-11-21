//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"testing"
)

func TestAntigravityCompatGeminiThinkingConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("streaming responses tier suffix enables includeThoughts without thinkingLevel", func(t *testing.T) {
		upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
		svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
		account := newAntigravityCompatAccount(AccountTypeOAuth)
		mapping, ok := account.Credentials["model_mapping"].(map[string]any)
		require.True(t, ok)
		mapping["gemini-4-argon-high"] = "gemini-4-argon-high"

		body := []byte(`{"model":"gemini-4-argon-high","stream":true,"input":"inspect repository"}`)
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)

		_, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Len(t, upstream.requestBodies, 1)

		cfg := gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig")
		require.True(t, cfg.Get("includeThoughts").Bool(), "thinkingConfig=%s", cfg.Raw)
		require.False(t, cfg.Get("thinkingLevel").Exists(), "tier-suffixed model must not set thinkingLevel: %s", cfg.Raw)
		require.False(t, cfg.Get("thinkingBudget").Exists(), "tier-suffixed model must not set thinkingBudget: %s", cfg.Raw)
	})

	t.Run("streaming chat completions tier suffix enables includeThoughts without thinkingLevel", func(t *testing.T) {
		upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
		svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
		account := newAntigravityCompatAccount(AccountTypeOAuth)
		mapping, ok := account.Credentials["model_mapping"].(map[string]any)
		require.True(t, ok)
		mapping["gemini-4-argon-high"] = "gemini-4-argon-high"

		body := []byte(`{"model":"gemini-4-argon-high","stream":true,"messages":[{"role":"user","content":"inspect repository"}]}`)
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)

		_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Len(t, upstream.requestBodies, 1)

		cfg := gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig")
		require.True(t, cfg.Get("includeThoughts").Bool(), "thinkingConfig=%s", cfg.Raw)
		require.False(t, cfg.Get("thinkingLevel").Exists(), "thinkingConfig=%s", cfg.Raw)
	})

	t.Run("streaming responses bare model with low effort selects low variant and enables includeThoughts", func(t *testing.T) {
		upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
		svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
		account := newAntigravityCompatAccount(AccountTypeOAuth)
		mapping, ok := account.Credentials["model_mapping"].(map[string]any)
		require.True(t, ok)
		mapping["gemini-4-argon-low"] = "gemini-4-argon-low"
		mapping["gemini-4-argon-high"] = "gemini-4-argon-high"

		body := []byte(`{"model":"gemini-4-argon","stream":true,"reasoning":{"effort":"low"},"input":"quick check"}`)
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)

		result, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Len(t, upstream.requestBodies, 1)
		require.Equal(t, "gemini-4-argon-low", gjson.GetBytes(upstream.requestBodies[0], "model").String())
		require.Equal(t, "gemini-4-argon-low", result.UpstreamModel)

		cfg := gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig")
		require.True(t, cfg.Get("includeThoughts").Bool(), "thinkingConfig=%s", cfg.Raw)
		require.False(t, cfg.Get("thinkingLevel").Exists(), "thinkingConfig=%s", cfg.Raw)
	})

	t.Run("streaming responses explicit bare model sets thinkingLevel and includeThoughts", func(t *testing.T) {
		upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
		svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
		account := newAntigravityCompatAccount(AccountTypeOAuth)
		mapping, ok := account.Credentials["model_mapping"].(map[string]any)
		require.True(t, ok)
		mapping["gemini-3.5-flash"] = "gemini-3.5-flash"

		body := []byte(`{"model":"gemini-3.5-flash","stream":true,"reasoning":{"effort":"medium"},"input":"check"}`)
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)

		_, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Len(t, upstream.requestBodies, 1)

		cfg := gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig")
		require.Equal(t, "medium", cfg.Get("thinkingLevel").String(), "thinkingConfig=%s", cfg.Raw)
		require.True(t, cfg.Get("includeThoughts").Bool(), "thinkingConfig=%s", cfg.Raw)
	})

	t.Run("non-streaming responses do not force includeThoughts", func(t *testing.T) {
		upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
		svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
		account := newAntigravityCompatAccount(AccountTypeOAuth)
		mapping, ok := account.Credentials["model_mapping"].(map[string]any)
		require.True(t, ok)
		mapping["gemini-4-argon-high"] = "gemini-4-argon-high"

		nonStreamBody := []byte(`{"model":"gemini-4-argon-high","stream":false,"input":"inspect"}`)
		c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nonStreamBody)
		_, err := svc.ForwardAsResponses(context.Background(), c, account, nonStreamBody, nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Len(t, upstream.requestBodies, 1)
		require.False(t, gjson.GetBytes(upstream.requestBodies[0], "request.generationConfig.thinkingConfig.includeThoughts").Bool())
	})
}
