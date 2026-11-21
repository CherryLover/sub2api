package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiNativeFallbackSessionStableAcrossAppendAndTailRewrite(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:generateContent", nil)
	h := &GatewayHandler{gatewayService: &service.GatewayService{}}
	first := []byte(`{"systemInstruction":{"parts":[{"text":"system"}]},"contents":[{"role":"user","parts":[{"text":"first"}]}]}`)
	tail := []byte(`{"systemInstruction":{"parts":[{"text":"system"}]},"contents":[{"role":"user","parts":[{"text":"first"}]},{"role":"model","parts":[{"text":"answer"}]},{"role":"user","parts":[{"text":"next with changed context"}]}]}`)
	a := h.geminiNativeFallbackSessionHash(c, 7, "gemini-4-carbon", first)
	require.NotEmpty(t, a)
	require.Equal(t, a, h.geminiNativeFallbackSessionHash(c, 7, "gemini-4-carbon", tail))
	require.NotEqual(t, a, h.geminiNativeFallbackSessionHash(c, 8, "gemini-4-carbon", tail))
	require.NotEqual(t, a, h.geminiNativeFallbackSessionHash(c, 7, "gemini-3.8-flash", tail))
}
