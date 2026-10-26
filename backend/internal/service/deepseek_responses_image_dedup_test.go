//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestDeepSeekResponsesCanonicalInputImageField(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolResponses}}
	for _, tc := range []struct{ name, part, want string }{
		{"duplicate url", `{"type":"input_image","image_url":"data:image/png;base64,AQID","url":"data:image/png;base64,AQID"}`, "data:image/png;base64,AQID"},
		{"prefer image_url", `{"type":"input_image","image_url":"data:image/png;base64,AQID","url":"https://invalid.example/img.png"}`, "data:image/png;base64,AQID"},
		{"url-only", `{"type":"input_image","url":"data:image/png;base64,BAUG"}`, "data:image/png;base64,BAUG"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"},` + tc.part + `]}]}`)
			got := normalizeDeepSeekResponsesRequestBody(account, body)
			require.Equal(t, tc.want, gjson.GetBytes(got, "input.0.content.1.image_url").String())
			require.False(t, gjson.GetBytes(got, "input.0.content.1.url").Exists())
			require.Equal(t, string(got), string(normalizeDeepSeekResponsesRequestBody(account, got)))
		})
	}
}
func TestDeepSeekResponsesImageDedupOtherPlatformsUnaffected(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AQID","url":"data:image/png;base64,AQID"}]}]}`)
	for _, platform := range []string{PlatformOpenAI, PlatformGemini, PlatformKimi} {
		account := &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolResponses}}
		require.Equal(t, string(body), string(normalizeDeepSeekResponsesRequestBody(account, body)))
	}
}
