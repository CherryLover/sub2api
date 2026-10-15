package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiExplicitCacheSchedulerProjection(t *testing.T) {
	acc := service.Account{ID: 42, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "test-api-key", "base_url": "https://relay.example.test", "explicit_cache_upstream": true, "explicit_cache_upstream_max_ttl_seconds": 600,
		"refresh_token": "must-not-be-projected", "access_token": "must-not-be-projected",
	}}
	snapshot := buildSchedulerMetadataAccount(acc)
	require.True(t, service.IsGeminiExplicitCacheAccount(&snapshot))
	require.Equal(t, 10*time.Minute, snapshot.GeminiExplicitCacheUpstreamMaxTTL())
	require.NotContains(t, snapshot.Credentials, "refresh_token")
	require.NotContains(t, snapshot.Credentials, "access_token")
	delete(acc.Credentials, "explicit_cache_upstream")
	snapshot = buildSchedulerMetadataAccount(acc)
	require.False(t, service.IsGeminiExplicitCacheAccount(&snapshot), "a relay must not turn into an official account when projecting credentials")
}
