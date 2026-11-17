package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAntigravityCarbonMappingUsesPublishedBaseModel(t *testing.T) {
	require.Equal(t, "gemini-4-carbon", DefaultAntigravityModelMapping["gemini-4-carbon"])
	require.Equal(t, "gemini-4-argon-high", DefaultAntigravityModelMapping["gemini-4-argon"])
}
