package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http/httptest"
	"testing"
)

func TestGeminiTruncatedStreamFailsTransportAfterMiddlewareCleanup(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "http1", true: "http2"}[http2], func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(middleware.Recovery())
			finished := make(chan struct{})
			router.Use(func(c *gin.Context) { c.Next(); close(finished) })
			router.GET("/stream", func(c *gin.Context) {
				c.Header("Content-Type", "text/event-stream")
				_, _ = c.Writer.WriteString("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n")
				c.Writer.Flush()
				(&GatewayHandler{}).handleGeminiFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: 502})
			})
			server := httptest.NewUnstartedServer(router)
			server.EnableHTTP2 = http2
			server.StartTLS()
			defer server.Close()
			resp, err := server.Client().Get(server.URL + "/stream")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			require.Contains(t, string(body), "partial")
			require.Error(t, err, "native clients must not mistake truncated Gemini content for a clean EOF")
			<-finished
		})
	}
}
