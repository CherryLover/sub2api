package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type catalogAccountRepoStub struct {
	AccountRepository
	visible    []Account
	catalog    []Account
	catalogErr error
}

func (r *catalogAccountRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	out := make([]Account, len(r.visible))
	copy(out, r.visible)
	return out, nil
}

func (r *catalogAccountRepoStub) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	if r.catalogErr != nil {
		return nil, r.catalogErr
	}
	src := r.catalog
	if src == nil {
		src = r.visible
	}
	out := make([]Account, len(src))
	copy(out, src)
	return out, nil
}

func decodeCodexManifestSlugs(t *testing.T, body []byte) []string {
	t.Helper()
	var envelope struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	slugs := make([]string, 0, len(envelope.Models))
	for _, model := range envelope.Models {
		slugs = append(slugs, model.Slug)
	}
	return slugs
}

func TestConfiguredCodexSupportsPriorityServiceTier(t *testing.T) {
	require.True(t, configuredCodexSupportsPriorityServiceTier("gpt-5.4"))
	require.True(t, configuredCodexSupportsPriorityServiceTier("gpt-5.5"))
	require.True(t, configuredCodexSupportsPriorityServiceTier("gpt-5.6-sol"))
	require.True(t, configuredCodexSupportsPriorityServiceTier("gpt-6-astra"))
	require.False(t, configuredCodexSupportsPriorityServiceTier("gpt-5.2"))

	descriptor := newConfiguredCodexModelDescriptor("gpt-5.4")
	require.Equal(t, []configuredCodexServiceTier{{
		ID:          OpenAIFastTierPriority,
		Name:        "Fast",
		Description: "Priority processing for lower latency.",
	}}, descriptor.ServiceTiers)
}

func TestConvertOpenAIModelListToCodexManifestForAccountMarksImageInput(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"gpt-5.4"},{"id":"custom-text-only"}]}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	converted := convertOpenAIModelListToCodexManifestForAccount(body, account)
	require.Equal(t, []string{"gpt-5.4", "custom-text-only"}, decodeCodexManifestSlugs(t, converted))

	var envelope struct {
		Models []struct {
			Slug            string   `json:"slug"`
			InputModalities []string `json:"input_modalities"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(converted, &envelope))
	require.Equal(t, []string{"text", "image"}, envelope.Models[0].InputModalities)
	require.Equal(t, []string{"text"}, envelope.Models[1].InputModalities)
}

func TestGroupCodexModelSupportsSearchToolRequiresAllOpenAICandidatesOnChatCompletions(t *testing.T) {
	ccAccount := Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Extra: map[string]any{"openai_responses_mode": "force_chat_completions"},
	}
	responsesAccount := Account{
		ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Extra: map[string]any{"openai_responses_mode": "force_responses"},
	}
	require.True(t, groupCodexModelSupportsSearchTool("gpt-5.4", []Account{ccAccount}))
	require.False(t, groupCodexModelSupportsSearchTool("gpt-5.4", []Account{ccAccount, responsesAccount}))
	require.False(t, groupCodexModelSupportsSearchTool("gpt-5.4", []Account{
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}))
}

func TestLoadCodexGroupCatalogAccountsUsesAvailabilityCandidates(t *testing.T) {
	visible := []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Schedulable: true}}
	catalog := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Schedulable: true},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Schedulable: false},
	}
	repo := &catalogAccountRepoStub{visible: visible, catalog: catalog}

	gotVisible, gotCatalog, err := loadCodexGroupCatalogAccounts(context.Background(), repo, 9)
	require.NoError(t, err)
	require.Len(t, gotVisible, 1)
	require.Len(t, gotCatalog, 2)
	require.Equal(t, int64(2), gotCatalog[1].ID)
}

func TestLoadCodexGroupCatalogAccountsFallsBackWhenAvailabilityQueryFails(t *testing.T) {
	visible := []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}}
	repo := &catalogAccountRepoStub{visible: visible, catalogErr: errors.New("availability query failed")}

	gotVisible, gotCatalog, err := loadCodexGroupCatalogAccounts(context.Background(), repo, 9)
	require.NoError(t, err)
	require.Equal(t, visible, gotVisible)
	require.Equal(t, visible, gotCatalog)
}

func TestBuildGroupConfiguredCodexModelsManifestUsesMappedAndDefaultModels(t *testing.T) {
	group := &Group{ID: 44, Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{
			ID:       2,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"glm-5.3": "glm-5.3"},
			},
		},
	}
	svc := &OpenAIGatewayService{accountRepo: &catalogAccountRepoStub{visible: accounts, catalog: accounts}}

	manifest, configured, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, "")
	require.NoError(t, err)
	require.True(t, configured)
	require.Contains(t, decodeCodexManifestSlugs(t, manifest.Body), "gpt-5.6-sol")
	require.Contains(t, decodeCodexManifestSlugs(t, manifest.Body), "glm-5.3")
	require.NotContains(t, decodeCodexManifestSlugs(t, manifest.Body), "gpt-image-2")
	require.Contains(t, string(manifest.Body), `"supported_reasoning_levels"`)
}

func TestBuildGroupConfiguredCodexModelsManifestHonorsCustomList(t *testing.T) {
	group := &Group{
		ID:       45,
		Platform: PlatformOpenAI,
		ModelsListConfig: GroupModelsListConfig{
			Enabled: true,
			Models:  []string{"gpt-5.6-sol", "unknown-model"},
		},
	}
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Credentials: map[string]any{"model_mapping": map[string]any{"team-coder": "gpt-5.6-sol"}},
		},
	}
	svc := &OpenAIGatewayService{accountRepo: &catalogAccountRepoStub{visible: accounts, catalog: accounts}}

	manifest, configured, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, "")
	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, []string{"gpt-5.6-sol"}, decodeCodexManifestSlugs(t, manifest.Body))
	require.Contains(t, openai.DefaultModelIDs(), "gpt-5.6-sol")
}

func TestBuildGroupConfiguredCodexModelsManifestReturnsFalseWithoutConfiguration(t *testing.T) {
	group := &Group{ID: 46, Platform: PlatformOpenAI}
	accounts := []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}}
	svc := &OpenAIGatewayService{accountRepo: &catalogAccountRepoStub{visible: accounts, catalog: accounts}}

	manifest, configured, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, "")
	require.NoError(t, err)
	require.False(t, configured)
	require.Nil(t, manifest)
}
