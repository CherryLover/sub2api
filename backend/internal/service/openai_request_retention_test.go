package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestViewScalarsOwnTheirMemory(t *testing.T) {
	body := []byte(`{"model":"gpt-5","prompt_cache_key":"session-1","previous_response_id":"resp-1","service_tier":"flex","reasoning":{"effort":"high"},"input":"` + strings.Repeat("x", 1<<20) + `"}`)
	view := newOpenAIRequestView(body)
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, "gpt-5", view.Model)
	require.Equal(t, "session-1", view.PromptCacheKey)
	require.Equal(t, "resp-1", view.PreviousResponseID)
	require.Equal(t, "flex", view.ServiceTier)
	require.Equal(t, "high", view.ReasoningEffort)
}

func TestOpenAIUsageLogDoesNotRetainRecordInputThroughFieldPointers(t *testing.T) {
	repo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(repo, nil, nil, nil)
	svc.cfg.RunMode = config.RunModeSimple
	input := &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "retention-test", Model: "gpt-5.1", Usage: OpenAIUsage{InputTokens: 10, OutputTokens: 5}, Duration: time.Second},
		APIKey: &APIKey{ID: 1000}, User: &User{ID: 2000}, Account: &Account{ID: 3000},
		UserAgent: "original-agent", IPAddress: "192.0.2.1",
	}
	require.NoError(t, svc.RecordUsage(context.Background(), input))
	require.NotNil(t, repo.lastLog)
	require.NotSame(t, &input.UserAgent, repo.lastLog.UserAgent)
	require.NotSame(t, &input.IPAddress, repo.lastLog.IPAddress)
	input.UserAgent = "changed-agent"
	input.IPAddress = "192.0.2.2"
	require.Equal(t, "original-agent", *repo.lastLog.UserAgent)
	require.Equal(t, "192.0.2.1", *repo.lastLog.IPAddress)
}
