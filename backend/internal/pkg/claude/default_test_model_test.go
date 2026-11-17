package claude

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDefaultTestModelUsesSupportedSonnet46(t *testing.T) {
	require.Equal(t, "claude-sonnet-4-6", DefaultTestModel)
	require.Contains(t, DefaultModelIDs(), DefaultTestModel)
	// Explicit legacy model routing remains available for configured accounts.
	require.Contains(t, DefaultModelIDs(), "claude-sonnet-4-5-20250929")
}
