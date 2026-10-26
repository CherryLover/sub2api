//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsGeminiImageGenerationIntent(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		models []string
		want   bool
	}{
		{name: "pro image", models: []string{"gemini-3-pro-image"}, want: true},
		{name: "flash image preview with prefix", models: []string{"models/gemini-3.1-flash-image-preview"}, want: true},
		{name: "image generation experimental", models: []string{"gemini-2.0-flash-exp-image-generation"}, want: true},
		{name: "imageless suffix", models: []string{"gemini-3-pro-imageless"}, want: false},
		{name: "image without gemini prefix", models: []string{"unknown-image", "my-image-chat"}, want: false},
		{name: "banana alias without resolved image model", models: []string{"nano-banana-2"}, want: false},
		{name: "any resolved name", models: []string{"my-alias", "gemini-2.5-flash-image"}, want: true},
		{name: "text model", models: []string{"gemini-3.7-flash", "gemini-3.1-pro-preview"}, want: false},
		{name: "empty", models: []string{"", "  "}, want: false},
		{name: "camelCase modalities", body: `{"generationConfig":{"responseModalities":["TEXT","IMAGE"]}}`, models: []string{"gemini-3.7-flash"}, want: true},
		{name: "snake_case modalities", body: `{"generation_config":{"response_modalities":["image"]}}`, want: true},
		{name: "mixed case keys", body: `{"generationConfig":{"response_modalities":[" Image "]}}`, want: true},
		{name: "text only modalities", body: `{"generationConfig":{"responseModalities":["TEXT"]}}`, models: []string{"gemini-3.7-flash"}, want: false},
		{name: "modalities string not array", body: `{"generationConfig":{"responseModalities":"IMAGE"}}`, want: true},
		{name: "no generation config", body: `{"contents":[{"parts":[{"text":"IMAGE"}]}]}`, want: false},
		{name: "integer enum modalities", body: `{"generationConfig":{"responseModalities":[1,2]}}`, want: true},
		{name: "integer enum text only", body: `{"generationConfig":{"responseModalities":[1]}}`, want: false},
		{name: "duplicate generationConfig keys", body: `{"generationConfig":{"responseModalities":["TEXT"]},"generationConfig":{"responseModalities":["IMAGE"]}}`, want: true},
		{name: "duplicate modalities keys", body: `{"generationConfig":{"responseModalities":["TEXT"],"responseModalities":["IMAGE"]}}`, want: true},
		{name: "invalid json", body: `{"generationConfig":{"responseModalities":["IMAGE"]`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isGeminiImageGenerationIntent([]byte(tc.body), tc.models...))
		})
	}
}

func TestGeminiImageGenerationDenied_GroupScope(t *testing.T) {
	cases := []struct {
		name  string
		group *Group
		want  bool
	}{
		{name: "ungrouped key", group: nil, want: false},
		{name: "gemini disabled", group: &Group{Platform: PlatformGemini}, want: true},
		{name: "antigravity disabled", group: &Group{Platform: PlatformAntigravity}, want: true},
		{name: "composite disabled", group: &Group{Platform: PlatformComposite}, want: true},
		{name: "gemini enabled", group: &Group{Platform: PlatformGemini, AllowImageGeneration: true}, want: false},
		{name: "anthropic group without the switch", group: &Group{Platform: PlatformAnthropic}, want: false},
		{name: "openai group gated elsewhere", group: &Group{Platform: PlatformOpenAI}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, GeminiImageGenerationDenied(tc.group, nil, "gemini-3-pro-image"))
		})
	}
}

func withGeminiImageGateAPIKey(c *gin.Context, group *Group) {
	c.Set("api_key", &APIKey{ID: 1, Group: group})
}

func imageDisabledGroup() *Group {
	return &Group{ID: 10, Platform: PlatformGemini, AllowImageGeneration: false}
}

func requireGeminiImagePermissionError(t *testing.T, rec *httptest.ResponseRecorder, wantGoogleStatus string) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errObj, ok := payload["error"].(map[string]any)
	require.True(t, ok, "error object: %s", rec.Body.String())
	require.Equal(t, ImageGenerationPermissionMessage(), errObj["message"])
	if wantGoogleStatus != "" {
		require.Equal(t, wantGoogleStatus, errObj["status"])
	} else {
		require.Equal(t, "invalid_request_error", errObj["type"])
	}
}

func newAntigravityImageGateAccount() *Account {
	account := newAntigravityCompatAccount(AccountTypeOAuth)
	account.Credentials["model_mapping"] = map[string]any{
		"gemini-3-pro-image": "gemini-3-pro-image",
		"my-draw":            "gemini-3.1-flash-image",
	}
	return account
}

func TestAntigravityForwards_ImageGenerationGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	geminiBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"draw a cat"}]}]}`)
	claudeBody := []byte(`{"model":"my-draw","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"draw a cat"}]}`)
	chatBody := []byte(`{"model":"gemini-3-pro-image","stream":true,"messages":[{"role":"user","content":"draw a cat"}]}`)

	forwards := []struct {
		name             string
		path             string
		body             []byte
		wantGoogleStatus string
		forward          func(*AntigravityGatewayService, *gin.Context, *Account) (*ForwardResult, error)
	}{
		{
			name:             "ForwardGemini",
			path:             "/v1beta/models/gemini-3-pro-image:streamGenerateContent",
			body:             geminiBody,
			wantGoogleStatus: "INVALID_ARGUMENT",
			forward: func(svc *AntigravityGatewayService, c *gin.Context, account *Account) (*ForwardResult, error) {
				return svc.ForwardGemini(context.Background(), c, account, "gemini-3-pro-image", "streamGenerateContent", true, geminiBody, false)
			},
		},
		{
			name: "Forward",
			path: "/v1/messages",
			body: claudeBody,
			forward: func(svc *AntigravityGatewayService, c *gin.Context, account *Account) (*ForwardResult, error) {
				return svc.Forward(context.Background(), c, account, claudeBody, false)
			},
		},
		{
			name: "ForwardAsChatCompletions",
			path: "/v1/chat/completions",
			body: chatBody,
			forward: func(svc *AntigravityGatewayService, c *gin.Context, account *Account) (*ForwardResult, error) {
				return svc.ForwardAsChatCompletions(context.Background(), c, account, chatBody, nil)
			},
		},
	}
	for _, fw := range forwards {
		t.Run(fw.name+"/disabled group rejects", func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			c, rec := newAntigravityCompatContext(http.MethodPost, fw.path, fw.body)
			withGeminiImageGateAPIKey(c, &Group{ID: 20, Platform: PlatformAntigravity, AllowImageGeneration: false})

			result, err := fw.forward(svc, c, newAntigravityImageGateAccount())
			require.Error(t, err)
			require.Nil(t, result)
			require.Zero(t, upstream.callCount)
			requireGeminiImagePermissionError(t, rec, fw.wantGoogleStatus)
		})
		t.Run(fw.name+"/enabled group reaches upstream", func(t *testing.T) {
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			c, rec := newAntigravityCompatContext(http.MethodPost, fw.path, fw.body)
			withGeminiImageGateAPIKey(c, &Group{ID: 21, Platform: PlatformAntigravity, AllowImageGeneration: true})

			_, _ = fw.forward(svc, c, newAntigravityImageGateAccount())
			require.Equal(t, 1, upstream.callCount)
			require.NotEqual(t, http.StatusBadRequest, rec.Code)
		})
	}
}
