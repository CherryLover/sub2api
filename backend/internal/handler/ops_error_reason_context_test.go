//go:build unit

package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyOpsErrorReasonFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		set  func(*gin.Context)
		want string
	}{
		{
			name: "context key missing",
			set:  func(*gin.Context) {},
			want: "existing",
		},
		{
			name: "context value is not a string",
			set: func(c *gin.Context) {
				c.Set(opsErrorReasonKey, 42)
			},
			want: "existing",
		},
		{
			name: "string value is trimmed",
			set: func(c *gin.Context) {
				c.Set(opsErrorReasonKey, "  model_rate_limited \t")
			},
			want: service.OpsNoAccountReasonModelRateLimited,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			tt.set(c)

			entry := &service.OpsInsertErrorLogInput{ErrorReason: "existing"}
			applyOpsErrorReasonFromContext(c, entry)
			require.Equal(t, tt.want, entry.ErrorReason)
		})
	}
}
