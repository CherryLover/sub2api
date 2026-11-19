package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
	"testing"
)

// Antigravity's Anthropic passthrough must retain cumulative TTL details even
// when the aggregate is absent or later frames contain explicit zero/null.
func TestAntigravitySparseCacheCreationTTLUsage(t *testing.T) {
	svc := &AntigravityGatewayService{}
	usage := &ClaudeUsage{}
	lines := []string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":200,"cache_creation":{"ephemeral_5m_input_tokens":400}}}}`,
		`data: {"type":"message_delta","usage":{"output_tokens":10,"cache_creation":{"ephemeral_1h_input_tokens":600}}}`,
		`data: {"type":"message_delta","usage":{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":null}}}`,
	}
	for _, line := range lines {
		svc.extractSSEUsage(line, usage)
	}
	require.Equal(t, 200, usage.InputTokens)
	require.Equal(t, 10, usage.OutputTokens)
	require.Equal(t, 400, usage.CacheCreation5mTokens)
	require.Equal(t, 600, usage.CacheCreation1hTokens)
	require.Equal(t, 1000, usage.effectiveCacheCreationTokens())

	direct := svc.extractClaudeUsage([]byte(`{"usage":{"input_tokens":200,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":400,"ephemeral_1h_input_tokens":600}}}`))
	require.Equal(t, 1000, direct.effectiveCacheCreationTokens())
	require.Equal(t, 400, direct.CacheCreation5mTokens)
	require.Equal(t, 600, direct.CacheCreation1hTokens)
}

func TestAntigravityCompatUsageKeepsInputAcrossOutputOnlyDelta(t *testing.T) {
	dst := &ClaudeUsage{}
	mergeAntigravityCompatUsage(dst, &antigravity.ClaudeUsage{InputTokens: 350, CacheReadInputTokens: 70, CacheCreationInputTokens: 30})
	mergeAntigravityCompatUsage(dst, &antigravity.ClaudeUsage{OutputTokens: 42})
	require.Equal(t, 350, dst.InputTokens)
	require.Equal(t, 70, dst.CacheReadInputTokens)
	require.Equal(t, 30, dst.CacheCreationInputTokens)
	require.Equal(t, 42, dst.OutputTokens)
}
