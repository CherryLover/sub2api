//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiImageGenerationRejected_BeforeAccountSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, platform, model, body string
		enabled, want               bool
	}{
		{"gemini disabled", service.PlatformGemini, "gemini-3-pro-image", "", false, true},
		{"antigravity disabled", service.PlatformAntigravity, "gemini-3-pro-image", "", false, true},
		{"composite disabled", service.PlatformComposite, "gemini-3-pro-image", "", false, true},
		{"mapped output image", service.PlatformGemini, "gemini-3.1-flash-image", "", false, true},
		{"native image modality", service.PlatformGemini, "gemini-3.7-flash", `{"generationConfig":{"responseModalities":["IMAGE"]}}`, false, true},
		{"text in disabled group", service.PlatformGemini, "gemini-3.7-flash", "", false, false},
		{"enabled group", service.PlatformGemini, "gemini-3-pro-image", "", true, false},
		{"anthropic group unchanged", service.PlatformAnthropic, "gemini-3-pro-image", "", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			key := &service.APIKey{Group: &service.Group{Platform: tt.platform, AllowImageGeneration: tt.enabled}}
			require.Equal(t, tt.want, geminiImageGenerationRejected(c, key, []byte(tt.body), tt.model))
		})
	}
}
