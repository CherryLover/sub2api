//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGPT61TerraLiteIdentityAndEffort(t *testing.T) {
	require.Contains(t, openai.DefaultModelIDs(), "gpt-6.1-terra")
	for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
		model := "openai/GPT-6.1-Terra-" + effort
		require.True(t, openai.IsGPT61TerraModelSpelling(model))
		require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel(model))
		require.Equal(t, effort, normalizeOpenAIReasoningEffortForModel(effort, model))
		req := &apicompat.AnthropicRequest{Model: model}
		applyOpenAICompatModelNormalization(req)
		require.Equal(t, "gpt-6.1-terra", req.Model)
		converted, err := apicompat.AnthropicToResponses(req)
		require.NoError(t, err)
		require.Equal(t, effort, converted.Reasoning.Effort)
	}
	for _, bad := range []string{"gpt-6.1-terra-ultra", "gpt-6.1-terra-minimal", "gpt-6.1-sol"} {
		require.False(t, openai.IsGPT61TerraModelSpelling(bad))
	}
	require.True(t, openai.RequiresResponsesForToolCalls("gpt-6.1-terra"))
	require.False(t, openai.RequiresResponsesForToolCalls("gpt-5.6-sol"))

	descriptor := newConfiguredCodexModelDescriptor("gpt-6.1-terra")
	require.NotNil(t, descriptor.DefaultReasoningLevel)
	require.Equal(t, "medium", *descriptor.DefaultReasoningLevel)
	var levels []string
	for _, level := range descriptor.SupportedReasoningLevels {
		levels = append(levels, level.Effort)
	}
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, levels)
}

func TestGPT61TerraLiteFallbackPricing(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	price := billing.getFallbackPricing("gpt-6.1-terra")
	require.NotNil(t, price)
	require.InDelta(t, 1e-6, price.InputPricePerToken, 1e-12)
	require.InDelta(t, 5e-6, price.OutputPricePerToken, 1e-12)
	require.InDelta(t, 0.05e-6, price.CacheReadPricePerToken, 1e-12)
	require.Equal(t, 272000, price.LongContextInputThreshold)
}

func TestGPT61TerraChatFallbackRejectsTools(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, toolField := range []string{`"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]`, `"functions":[{"name":"lookup","parameters":{"type":"object"}}]`} {
			if protocol != "chat" && strings.HasPrefix(toolField, `"functions"`) {
				continue
			}
			body := `{"model":"public","reasoning_effort":"none","messages":[{"role":"user","content":"hi"}],"input":"hi","max_tokens":100,` + toolField + `}`
			if protocol == "responses" {
				body = `{"model":"public","input":"hi","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
			}
			if protocol == "messages" {
				body = `{"model":"public","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`
			}
			t.Run(protocol+"/"+toolField, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ginContext, _ := gin.CreateTestContext(recorder)
				ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/"+protocol, strings.NewReader(body))
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": "gpt-6.1-terra"}}}
				service := &OpenAIGatewayService{cfg: &config.Config{}}
				var err error
				switch protocol {
				case "chat":
					_, err = service.forwardAsRawChatCompletions(context.Background(), ginContext, account, []byte(body), "")
				case "responses":
					_, err = service.forwardResponsesViaRawChatCompletions(context.Background(), ginContext, account, []byte(body), time.Now())
				case "messages":
					_, err = service.forwardAnthropicViaRawChatCompletions(context.Background(), ginContext, account, []byte(body), "")
				}
				require.ErrorContains(t, err, "gpt-6.1-terra requires Responses")
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			})
		}
	}
}
