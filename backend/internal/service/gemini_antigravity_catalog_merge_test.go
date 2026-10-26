//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type agCatalogRepoStub struct {
	AccountRepository
	accounts []Account
}

func (s *agCatalogRepoStub) ListSchedulableByGroupIDAndPlatforms(_ context.Context, _ int64, _ []string) ([]Account, error) {
	return s.accounts, nil
}

func TestAntigravityGeminiModelIDsRespectMixedScheduling(t *testing.T) {
	groupID := int64(9)
	repo := &agCatalogRepoStub{accounts: []Account{
		{ID: 1, Platform: PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-3.1-pro-preview": "actual-gemini-model", "claude-sonnet-4-6": "thinking-model"}}},
		{ID: 2, Platform: PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": false}, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-2.5-flash": "upstream"}}},
		{ID: 3, Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-2.5-pro": "upstream"}}},
	}}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	mixed, err := svc.AntigravityGeminiModelIDs(context.Background(), &groupID, true)
	require.NoError(t, err)
	require.Contains(t, mixed, "gemini-3.1-pro-preview")
	require.NotContains(t, mixed, "gemini-2.5-flash")
	forced, err := svc.AntigravityGeminiModelIDs(context.Background(), &groupID, false)
	require.NoError(t, err)
	require.Contains(t, forced, "gemini-3.1-pro-preview")
	require.Contains(t, forced, "gemini-2.5-flash")
}
