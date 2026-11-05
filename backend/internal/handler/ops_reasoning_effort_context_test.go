//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyOpsReasoningEffortFromContext(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "missing"},
		{name: "wrong type", value: 42},
		{name: "empty", value: "  "},
		{name: "trimmed", value: " high ", want: "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			if tc.value != nil {
				c.Set(opsReasoningEffortKey, tc.value)
			}
			entry := &service.OpsInsertErrorLogInput{}

			applyOpsReasoningEffortFromContext(c, entry)

			if tc.want == "" {
				require.Nil(t, entry.ReasoningEffort)
				return
			}
			require.NotNil(t, entry.ReasoningEffort)
			require.Equal(t, tc.want, *entry.ReasoningEffort)
		})
	}
}

func TestSetOpsClaudeReasoningEffortUsesParsedThinkingFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)

	setOpsClaudeReasoningEffort(c, "glm-5", "", true)

	value, ok := c.Get(opsReasoningEffortKey)
	require.True(t, ok)
	require.Equal(t, "high", value)
}
