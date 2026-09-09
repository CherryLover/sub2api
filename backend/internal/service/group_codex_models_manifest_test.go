//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexModelsManifestConfig(t *testing.T) {
	t.Run("non openai platforms are zeroed", func(t *testing.T) {
		got := normalizeCodexModelsManifestConfig(PlatformAnthropic, GroupCodexModelsManifestConfig{
			Enabled:             true,
			AccountIDs:          []int64{1, 2},
			FallbackToScheduler: true,
		})
		require.Equal(t, GroupCodexModelsManifestConfig{}, got)
	})

	t.Run("dedupes and drops invalid ids while keeping disabled selection", func(t *testing.T) {
		got := normalizeCodexModelsManifestConfig(PlatformOpenAI, GroupCodexModelsManifestConfig{
			Enabled:             false,
			AccountIDs:          []int64{0, 11, 11, -3, 22, 0, 11},
			FallbackToScheduler: true,
		})
		require.Equal(t, GroupCodexModelsManifestConfig{
			Enabled:             false,
			AccountIDs:          []int64{11, 22},
			FallbackToScheduler: true,
		}, got)
	})
}

func TestAdminService_CreateGroup_CodexModelsManifestConfigEnabledRejected(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "codex-manifest",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1,
		CodexModelsManifestConfig: GroupCodexModelsManifestConfig{
			Enabled:    true,
			AccountIDs: []int64{1},
		},
	})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "INVALID_CODEX_MODELS_MANIFEST_CONFIG", infraerrors.Reason(err))
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_CodexModelsManifestConfigDisabledAccepted(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "codex-manifest",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1,
		CodexModelsManifestConfig: GroupCodexModelsManifestConfig{
			Enabled:             false,
			AccountIDs:          []int64{0, 9, 9},
			FallbackToScheduler: true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Equal(t, GroupCodexModelsManifestConfig{
		Enabled:             false,
		AccountIDs:          []int64{9},
		FallbackToScheduler: true,
	}, repo.created.CodexModelsManifestConfig)
}

func TestAdminService_UpdateGroup_CodexModelsManifestConfigZeroedForNonOpenAIPlatform(t *testing.T) {
	existing := &Group{
		ID:       7,
		Name:     "anthropic-group",
		Platform: PlatformAnthropic,
		Status:   StatusActive,
		CodexModelsManifestConfig: GroupCodexModelsManifestConfig{
			Enabled:    true,
			AccountIDs: []int64{1},
		},
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{
		Name: "anthropic-group",
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Equal(t, GroupCodexModelsManifestConfig{}, repo.updated.CodexModelsManifestConfig)
}

func TestAdminService_UpdateGroup_CodexModelsManifestConfigUntouchedWhenOmitted(t *testing.T) {
	existing := &Group{
		ID:       8,
		Name:     "openai-group",
		Platform: PlatformOpenAI,
		Status:   StatusActive,
		CodexModelsManifestConfig: GroupCodexModelsManifestConfig{
			Enabled:    true,
			AccountIDs: []int64{101, 202},
		},
	}
	repo := &groupRepoStubForAdmin{getByID: existing}
	svc := &adminServiceImpl{groupRepo: repo}

	name := "openai-group-renamed"
	group, err := svc.UpdateGroup(context.Background(), 8, &UpdateGroupInput{
		Name: name,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Equal(t, GroupCodexModelsManifestConfig{
		Enabled:    true,
		AccountIDs: []int64{101, 202},
	}, repo.updated.CodexModelsManifestConfig)
}
