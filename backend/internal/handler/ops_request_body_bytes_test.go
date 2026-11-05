package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsRequestBodyCounterRecordsBytesReadByHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := strings.Repeat("x", 4096)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))

	installOpsRequestBodyCounter(c)
	read, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	require.Len(t, read, len(body))

	entry := &service.OpsInsertErrorLogInput{}
	applyOpsRequestBodyBytesFromContext(c, entry)
	require.NotNil(t, entry.RequestBodyBytes)
	require.EqualValues(t, len(body), *entry.RequestBodyBytes)
}

func TestOpsRequestBodyCounterSkipsRequestsWithoutBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	installOpsRequestBodyCounter(c)
	require.Equal(t, http.NoBody, c.Request.Body)

	entry := &service.OpsInsertErrorLogInput{}
	applyOpsRequestBodyBytesFromContext(c, entry)
	require.Nil(t, entry.RequestBodyBytes)
}

func TestOpsRequestBodyCounterLeavesUnreadBodyUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{}"))

	installOpsRequestBodyCounter(c)

	entry := &service.OpsInsertErrorLogInput{}
	applyOpsRequestBodyBytesFromContext(c, entry)
	require.Nil(t, entry.RequestBodyBytes, "an auth rejection before reading the body records no size")
}

func TestOpsErrorLoggerMiddlewareInstallsRequestBodyCounter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(nil))

	var recorded *int64
	router.POST("/v1/responses", func(c *gin.Context) {
		_, err := io.Copy(io.Discard, c.Request.Body)
		require.NoError(t, err)
		entry := &service.OpsInsertErrorLogInput{}
		applyOpsRequestBodyBytesFromContext(c, entry)
		recorded = entry.RequestBodyBytes
		c.Status(http.StatusBadGateway)
	})

	payload := strings.Repeat("y", 3<<20)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload)))

	require.NotNil(t, recorded)
	require.EqualValues(t, len(payload), *recorded)
}

func TestOpsRequestBodyCounterCountsAcrossPartialReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("0123456789"))
	installOpsRequestBodyCounter(c)

	buf := make([]byte, 3)
	for {
		if _, err := c.Request.Body.Read(buf); err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
	}
	require.NoError(t, c.Request.Body.Close())

	entry := &service.OpsInsertErrorLogInput{}
	applyOpsRequestBodyBytesFromContext(c, entry)
	require.NotNil(t, entry.RequestBodyBytes)
	require.EqualValues(t, 10, *entry.RequestBodyBytes)
}
