package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestSupplementUnmappedOpenAIModels(t *testing.T) {
	mapped := []string{"team-coder"}
	mappedOnly := []Account{{
		Platform:    PlatformOpenAI,
		Credentials: map[string]any{"model_mapping": map[string]any{"team-coder": "gpt-5.6-sol"}},
	}}
	require.Equal(t, mapped, supplementUnmappedOpenAIModels(mappedOnly, mapped))

	unmapped := []Account{{Platform: PlatformOpenAI}}
	require.Empty(t, supplementUnmappedOpenAIModels(unmapped, nil))

	got := supplementUnmappedOpenAIModels(unmapped, mapped)
	require.Contains(t, got, "team-coder")
	require.Contains(t, got, openai.DefaultModelIDs()[0])

	otherPlatform := []Account{{Platform: PlatformAnthropic}}
	require.Equal(t, mapped, supplementUnmappedOpenAIModels(otherPlatform, mapped))
}
