package claude

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHaiku55ModelAndEffortAliases(t *testing.T) {
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, EffortLevelsForModel("claude-haiku-5-5"))
	for _, model := range []string{"claude-haiku-5-5", "anthropic/claude-haiku-5.5", "us.anthropic.claude-haiku-5-5", "global.anthropic.claude-haiku-5-5-thinking"} {
		require.True(t, IsHaiku55(model), model)
	}
	for _, model := range []string{"claude-haiku-5", "claude-haiku-4-5", "claude-sonnet-5-5"} {
		require.False(t, IsHaiku55(model), model)
	}
}
