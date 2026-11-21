package antigravity

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiUnspecifiedBlockIsNotTerminal(t *testing.T) {
	for _, v := range []string{"BLOCK_REASON_UNSPECIFIED", "BLOCKED_REASON_UNSPECIFIED", ""} {
		require.False(t, isTerminalGeminiBlockReason(v))
	}
	require.True(t, isTerminalGeminiBlockReason("SAFETY"))
}
