package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	configuredCodexModelPriority       = 50
	configuredCodexCustomDescription   = "Custom model routed through Sub2API."
	configuredCodexFallbackContext     = 272_000
	configuredCodexDeepSeekV4Context   = 1_000_000
	configuredCodexGrokContext         = 500_000
	configuredCodexGrokBuildContext    = 256_000
	configuredCodexGPT56MaxContext     = 872_000
	configuredCodexGPT6AstraContext    = 1_050_000
	configuredCodexToolOutputMaxTokens = 10_000
	codexAutoModelPrefix               = "codex-auto-"
)

type configuredCodexReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type configuredCodexTruncationPolicy struct {
	Mode  string `json:"mode"`
	Limit int64  `json:"limit"`
}

type configuredCodexServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type configuredCodexModelMessages struct {
	InstructionsTemplate  string `json:"instructions_template"`
	InstructionsVariables any    `json:"instructions_variables"`
	Approvals             any    `json:"approvals"`
	CollaborationModes    any    `json:"collaboration_modes"`
	AutoReview            any    `json:"auto_review"`
	Permissions           any    `json:"permissions"`
	MultiAgent            any    `json:"multi_agent"`
	TokenBudget           any    `json:"token_budget"`
	GuardianV2            any    `json:"guardian_v2"`
}

// configuredCodexModelDescriptor is the minimum complete ModelInfo contract
// understood by current Codex clients.
type configuredCodexModelDescriptor struct {
	Slug                              string                          `json:"slug"`
	DisplayName                       string                          `json:"display_name"`
	Description                       string                          `json:"description"`
	DefaultReasoningLevel             *string                         `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels          []configuredCodexReasoningLevel `json:"supported_reasoning_levels"`
	MultiAgentReasoningEffort         *string                         `json:"multi_agent_reasoning_effort,omitempty"`
	ShellType                         string                          `json:"shell_type"`
	Visibility                        string                          `json:"visibility"`
	SupportedInAPI                    bool                            `json:"supported_in_api"`
	Priority                          int                             `json:"priority"`
	AdditionalSpeedTiers              []string                        `json:"additional_speed_tiers"`
	ServiceTiers                      []configuredCodexServiceTier    `json:"service_tiers"`
	DefaultServiceTier                any                             `json:"default_service_tier"`
	AvailabilityNUX                   any                             `json:"availability_nux"`
	Upgrade                           any                             `json:"upgrade"`
	ModelMessages                     configuredCodexModelMessages    `json:"model_messages"`
	IncludeSkillsUsageInstructions    bool                            `json:"include_skills_usage_instructions"`
	IncludePluginUsageInstructions    bool                            `json:"include_plugin_usage_instructions"`
	IncludeAppsUsageInstructions      bool                            `json:"include_apps_usage_instructions"`
	SupportsReasoningSummaryParameter bool                            `json:"supports_reasoning_summary_parameter"`
	DefaultReasoningSummary           string                          `json:"default_reasoning_summary"`
	SupportVerbosity                  bool                            `json:"support_verbosity"`
	DefaultVerbosity                  *string                         `json:"default_verbosity"`
	ApplyPatchToolType                *string                         `json:"apply_patch_tool_type"`
	WebSearchToolType                 string                          `json:"web_search_tool_type"`
	TruncationPolicy                  configuredCodexTruncationPolicy `json:"truncation_policy"`
	SupportsImageDetailOriginal       bool                            `json:"supports_image_detail_original"`
	SupportsParallelToolCalls         bool                            `json:"supports_parallel_tool_calls"`
	ContextWindow                     int64                           `json:"context_window"`
	MaxContextWindow                  int64                           `json:"max_context_window"`
	AutoCompactTokenLimit             any                             `json:"auto_compact_token_limit"`
	CompHash                          any                             `json:"comp_hash"`
	EffectiveContextWindowPercent     int64                           `json:"effective_context_window_percent"`
	ExperimentalSupportedTools        []string                        `json:"experimental_supported_tools"`
	InputModalities                   []string                        `json:"input_modalities"`
	SupportsSearchTool                bool                            `json:"supports_search_tool"`
	UseResponsesLite                  bool                            `json:"use_responses_lite"`
	NodeREPLAutoReviewRequired        bool                            `json:"node_repl_auto_review_required"`
	NodeREPLDisabled                  bool                            `json:"node_repl_disabled"`
	AutoReviewModelOverride           any                             `json:"auto_review_model_override"`
	ModelSpecialty                    any                             `json:"model_specialty"`
	ToolMode                          any                             `json:"tool_mode"`
	MultiAgentVersion                 any                             `json:"multi_agent_version"`
}

// FilterCodexModelIDsForGroup removes dedicated media-generation models,
// wildcard mapping keys, and Codex automatic modes from a client catalog.
func FilterCodexModelIDsForGroup(modelIDs []string, group *Group) []string {
	explicitlyEnabled := make(map[string]struct{})
	if group != nil && group.CustomModelsListEnabled() {
		for _, modelID := range group.ModelsListConfig.Models {
			modelID = strings.TrimSpace(modelID)
			if strings.HasPrefix(modelID, codexAutoModelPrefix) {
				explicitlyEnabled[modelID] = struct{}{}
			}
		}
	}

	filtered := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" || strings.Contains(modelID, "*") {
			continue
		}
		if isCodexDedicatedMediaModel(modelID) {
			continue
		}
		if strings.HasPrefix(modelID, codexAutoModelPrefix) {
			if _, ok := explicitlyEnabled[modelID]; !ok {
				continue
			}
		}
		filtered = append(filtered, modelID)
	}
	if group != nil && group.CustomModelsListEnabled() {
		return filterCodexModelsByCustomList(filtered, group.ModelsListConfig.Models)
	}
	return filtered
}

func filterCodexModelsByCustomList(available, selected []string) []string {
	index := make(map[string]struct{}, len(available))
	for _, modelID := range available {
		index[modelID] = struct{}{}
	}
	out := make([]string, 0, len(selected))
	seen := make(map[string]struct{}, len(selected))
	for _, modelID := range selected {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if _, ok := index[modelID]; !ok {
			continue
		}
		if _, dup := seen[modelID]; dup {
			continue
		}
		seen[modelID] = struct{}{}
		out = append(out, modelID)
	}
	return out
}

func isCodexDedicatedMediaModel(modelID string) bool {
	canonical := codexProviderQualifiedModelID(modelID)
	return IsGPTImageGenerationModel(canonical) || isImageGenerationModel(canonical)
}

func codexProviderQualifiedModelID(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if slash := strings.LastIndexByte(modelID, '/'); slash >= 0 {
		modelID = strings.TrimSpace(modelID[slash+1:])
	}
	return strings.TrimPrefix(modelID, "models/")
}

// BuildGroupConfiguredCodexModelsManifest builds a Codex catalog from configured
// public model names, supplemented by defaults for unmapped OpenAI accounts.
func (s *OpenAIGatewayService) BuildGroupConfiguredCodexModelsManifest(
	ctx context.Context,
	group *Group,
	ifNoneMatch string,
) (*CodexModelsManifest, bool, error) {
	if s == nil || s.accountRepo == nil || group == nil || group.Platform != PlatformOpenAI {
		return nil, false, nil
	}

	visible, catalog, err := loadCodexGroupCatalogAccounts(ctx, s.accountRepo, group.ID)
	if err != nil {
		return nil, false, fmt.Errorf("load group configured Codex models: %w", err)
	}
	configuredModels := openAIConfiguredCodexModelIDsForGroup(visible, group)
	if len(configuredModels) == 0 {
		return nil, false, nil
	}

	body, err := buildCodexModelsManifestForAccounts(configuredModels, catalog)
	if err != nil {
		return nil, false, fmt.Errorf("initialize group configured Codex models: %w", err)
	}
	manifest := &CodexModelsManifest{
		Body: body,
		ETag: codexModelsManifestBodyETag(body),
	}
	if codexModelsManifestETagMatches(ifNoneMatch, manifest.ETag) {
		manifest.Body = nil
		manifest.NotModified = true
	}
	return manifest, true, nil
}

// loadCodexGroupCatalogAccounts separates picker membership from capability
// intersection. visible accounts are currently schedulable and decide which
// public aliases appear. catalog accounts are persistently enabled group
// members; the availability query ignores transient rate-limit, overload, and
// temporary-unschedulable state so those conditions cannot widen advertised
// capabilities. Persistently disabled accounts are excluded because routing
// cannot select them. If the availability query fails, the catalog falls back
// to the schedulable set so a listing error does not fail the client request.
func loadCodexGroupCatalogAccounts(ctx context.Context, repo AccountRepository, groupID int64) (visible []Account, catalog []Account, err error) {
	if repo == nil {
		return nil, nil, nil
	}
	visible, err = repo.ListSchedulableByGroupID(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	catalog = visible
	groupAccounts, listErr := repo.ListModelAvailabilityCandidates(
		ctx,
		&groupID,
		[]string{
			PlatformAnthropic,
			PlatformOpenAI,
			PlatformGemini,
			PlatformAntigravity,
			PlatformGrok,
			PlatformKimi,
			PlatformZhipu,
			PlatformDeepseek,
		},
		false,
	)
	if listErr != nil {
		return visible, catalog, nil
	}
	return visible, groupAccounts, nil
}

func openAIConfiguredCodexModelIDs(accounts []Account) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformOpenAI {
			continue
		}
		for modelID := range account.GetModelMapping() {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" || strings.Contains(modelID, "*") {
				continue
			}
			if _, exists := seen[modelID]; exists {
				continue
			}
			seen[modelID] = struct{}{}
			models = append(models, modelID)
		}
	}
	sort.Strings(models)
	return models
}

func openAIConfiguredCodexModelIDsForGroup(accounts []Account, group *Group) []string {
	models := supplementUnmappedOpenAIModels(accounts, openAIConfiguredCodexModelIDs(accounts))
	return FilterCodexModelIDsForGroup(models, group)
}

func newConfiguredCodexModelDescriptor(modelID string) configuredCodexModelDescriptor {
	modelID = strings.TrimSpace(modelID)
	noReasoningLevel := "none"
	descriptor := configuredCodexModelDescriptor{
		Slug:                  modelID,
		DisplayName:           modelID,
		Description:           configuredCodexCustomDescription,
		DefaultReasoningLevel: &noReasoningLevel,
		SupportedReasoningLevels: []configuredCodexReasoningLevel{
			{Effort: "none", Description: configuredCodexReasoningLevelDescription("none")},
		},
		ShellType:                         "unified_exec",
		Visibility:                        "list",
		SupportedInAPI:                    true,
		Priority:                          configuredCodexModelPriority,
		AdditionalSpeedTiers:              []string{},
		ServiceTiers:                      []configuredCodexServiceTier{},
		ModelMessages:                     configuredCodexModelMessages{InstructionsTemplate: openai.CodexBaseInstructionsForModel(modelID)},
		SupportsReasoningSummaryParameter: true,
		DefaultReasoningSummary:           "auto",
		WebSearchToolType:                 "text",
		TruncationPolicy:                  configuredCodexTruncationPolicy{Mode: "bytes", Limit: configuredCodexToolOutputMaxTokens},
		ContextWindow:                     configuredCodexFallbackContext,
		MaxContextWindow:                  configuredCodexFallbackContext,
		EffectiveContextWindowPercent:     95,
		ExperimentalSupportedTools:        []string{},
		InputModalities:                   []string{"text"},
	}

	if isDeepSeekCodexModel(modelID) {
		defaultReasoningLevel := "high"
		descriptor.DisplayName = deepSeekCodexDisplayName(modelID)
		descriptor.Description = "DeepSeek coding and reasoning model routed through Sub2API."
		descriptor.DefaultReasoningLevel = &defaultReasoningLevel
		descriptor.SupportedReasoningLevels = []configuredCodexReasoningLevel{
			{Effort: "low", Description: "Fast responses with lighter reasoning"},
			{Effort: "high", Description: "Greater reasoning depth for coding and agent tasks"},
			{Effort: "max", Description: "Maximum reasoning depth for complex tasks"},
		}
		descriptor.SupportsParallelToolCalls = true
		descriptor.ContextWindow = configuredCodexDeepSeekV4Context
		descriptor.MaxContextWindow = configuredCodexDeepSeekV4Context
	}

	if isGrokCodexModel(modelID) {
		descriptor.DisplayName = grokCodexDisplayName(modelID)
		descriptor.Description = "Grok coding and reasoning model routed through Sub2API."
		descriptor.SupportsParallelToolCalls = true
		descriptor.ContextWindow = grokCodexContextWindow(modelID)
		descriptor.MaxContextWindow = descriptor.ContextWindow
		if grokSupportsReasoningEffort(modelID) {
			defaultReasoningLevel := "high"
			descriptor.DefaultReasoningLevel = &defaultReasoningLevel
			descriptor.SupportedReasoningLevels = configuredCodexGrokReasoningLevels()
		}
	}

	if isClaudeCodexModel(modelID) {
		descriptor.DisplayName = claudeCodexDisplayName(modelID)
		descriptor.Description = "Claude coding and reasoning model routed through Sub2API."
		descriptor.SupportsParallelToolCalls = true
	}

	if isOpenAICodexGPTModel(modelID) {
		descriptor.DisplayName = openaiCodexDisplayName(modelID)
		descriptor.Description = "OpenAI GPT coding model routed through Sub2API."
		descriptor.SupportsParallelToolCalls = true
		descriptor.ServiceTiers = configuredCodexServiceTiersForModel(modelID)
		if isOpenAICodexReasoningGPTModel(modelID) {
			defaultReasoningLevel := "medium"
			if getNormalizedCodexModel(modelID) == "gpt-5.6-sol" {
				defaultReasoningLevel = "low"
			}
			descriptor.DefaultReasoningLevel = &defaultReasoningLevel
			descriptor.SupportedReasoningLevels = configuredCodexGPTReasoningLevels(modelID)
			descriptor.DefaultReasoningSummary = "none"
			descriptor.TruncationPolicy = configuredCodexTruncationPolicy{Mode: "tokens", Limit: configuredCodexToolOutputMaxTokens}
			if isOpenAIGPT56Model(modelID) {
				descriptor.MaxContextWindow = configuredCodexGPT56MaxContext
			}
			if isOpenAIGPT6AstraModel(modelID) {
				multiAgentEffort := "xhigh"
				descriptor.MultiAgentReasoningEffort = &multiAgentEffort
				descriptor.MultiAgentVersion = "v2"
				descriptor.ContextWindow = configuredCodexGPT6AstraContext
				descriptor.MaxContextWindow = configuredCodexGPT6AstraContext
			}
		}
		if SupportsVerbosity(modelID) {
			defaultVerbosity := "low"
			descriptor.SupportVerbosity = true
			descriptor.DefaultVerbosity = &defaultVerbosity
		}
	}

	return descriptor
}

func configuredCodexReasoningLevelDescription(effort string) string {
	switch effort {
	case "none":
		return "No extra reasoning"
	case "low":
		return "Fast responses with lighter reasoning"
	case "medium":
		return "Balanced reasoning for most coding tasks"
	case "high":
		return "Greater reasoning depth for coding and agent tasks"
	case "xhigh":
		return "Extra-high reasoning depth for difficult tasks"
	case "max":
		return "Maximum reasoning depth for complex tasks"
	case "ultra":
		return "Maximum reasoning with automatic task delegation"
	default:
		return effort
	}
}

func configuredCodexServiceTiersForModel(modelID string) []configuredCodexServiceTier {
	if !configuredCodexSupportsPriorityServiceTier(modelID) {
		return []configuredCodexServiceTier{}
	}
	return []configuredCodexServiceTier{
		{
			ID:          OpenAIFastTierPriority,
			Name:        "Fast",
			Description: "Priority processing for lower latency.",
		},
	}
}

func configuredCodexSupportsPriorityServiceTier(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	for _, family := range []string{"gpt-5.4", "gpt-5.5", "gpt-5.6"} {
		if normalized == family || strings.HasPrefix(normalized, family+"-") {
			return true
		}
	}
	return isOpenAIGPT6AstraModel(modelID)
}

func configuredCodexGrokReasoningLevels() []configuredCodexReasoningLevel {
	return []configuredCodexReasoningLevel{
		{Effort: "low", Description: configuredCodexReasoningLevelDescription("low")},
		{Effort: "medium", Description: configuredCodexReasoningLevelDescription("medium")},
		{Effort: "high", Description: configuredCodexReasoningLevelDescription("high")},
	}
}

func configuredCodexGPTReasoningLevels(modelID string) []configuredCodexReasoningLevel {
	levels := []configuredCodexReasoningLevel{
		{Effort: "low", Description: configuredCodexReasoningLevelDescription("low")},
		{Effort: "medium", Description: configuredCodexReasoningLevelDescription("medium")},
		{Effort: "high", Description: configuredCodexReasoningLevelDescription("high")},
		{Effort: "xhigh", Description: configuredCodexReasoningLevelDescription("xhigh")},
	}
	normalized := getNormalizedCodexModel(modelID)
	if isOpenAIGPT56Model(modelID) || isOpenAIGPT6AstraModel(modelID) {
		levels = append(levels, configuredCodexReasoningLevel{
			Effort:      "max",
			Description: configuredCodexReasoningLevelDescription("max"),
		})
	}
	if isOpenAIGPT6AstraModel(modelID) || normalized == "gpt-5.6-sol" || normalized == "gpt-5.6-terra" {
		levels = append(levels, configuredCodexReasoningLevel{
			Effort:      "ultra",
			Description: configuredCodexReasoningLevelDescription("ultra"),
		})
	}
	return levels
}

func isOpenAICodexGPTModel(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	if normalized == "" || strings.HasPrefix(normalized, "gpt-image") {
		return false
	}
	return strings.HasPrefix(normalized, "gpt-")
}

func isOpenAICodexReasoningGPTModel(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	return isOpenAIGPT6AstraModel(normalized) || strings.HasPrefix(normalized, "gpt-5")
}

func isOpenAICodexImageInputModel(modelID string) bool {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	return isOpenAIGPT6AstraModel(normalized) ||
		strings.HasPrefix(normalized, "gpt-5") ||
		strings.HasPrefix(normalized, "gpt-4o") ||
		strings.HasPrefix(normalized, "gpt-4.1") ||
		strings.HasPrefix(normalized, "gpt-4.5") ||
		strings.HasPrefix(normalized, "gpt-4-turbo") ||
		strings.HasPrefix(normalized, "gpt-4-vision")
}

func openaiCodexDisplayName(modelID string) string {
	normalized := canonicalizeOpenAIModelAliasSpelling(modelID)
	if normalized == "" {
		return modelID
	}
	for _, model := range openai.DefaultModels {
		if strings.EqualFold(model.ID, normalized) && strings.TrimSpace(model.DisplayName) != "" {
			return model.DisplayName
		}
	}
	return modelID
}

func deepSeekCodexDisplayName(modelID string) string {
	switch strings.ToLower(strings.TrimSpace(modelID)) {
	case "deepseek-v4-pro", "deepseek-4-pro":
		return "DeepSeek V4 Pro"
	case "deepseek-v4-flash", "deepseek-4-flash":
		return "DeepSeek V4 Flash"
	default:
		return modelID
	}
}

func isDeepSeekCodexModel(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "deepseek-")
}

func isGrokCodexModel(modelID string) bool {
	return xai.IsGrokModelID(modelID)
}

func grokCodexDisplayName(modelID string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	for _, model := range xai.DefaultModels() {
		if strings.EqualFold(model.ID, normalized) && strings.TrimSpace(model.DisplayName) != "" {
			return model.DisplayName
		}
	}
	return modelID
}

func grokCodexContextWindow(modelID string) int64 {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if strings.HasPrefix(normalized, "grok-build") {
		return configuredCodexGrokBuildContext
	}
	return configuredCodexGrokContext
}

func isClaudeCodexModel(modelID string) bool {
	platform, detected := DetectModelPlatform(modelID)
	return detected && platform == PlatformAnthropic
}

func claudeCodexDisplayName(modelID string) string {
	normalized := strings.ToLower(codexProviderQualifiedModelID(modelID))
	normalized = strings.TrimPrefix(normalized, "anthropic.")
	if normalized == "" {
		return modelID
	}
	for _, model := range claude.DefaultModels {
		if strings.EqualFold(model.ID, normalized) && strings.TrimSpace(model.DisplayName) != "" {
			return model.DisplayName
		}
	}
	return modelID
}

func buildCodexModelsManifestForAccounts(modelIDs []string, accounts []Account) ([]byte, error) {
	imageInputModels := make(map[string]bool, len(modelIDs))
	searchToolModels := make(map[string]bool, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if groupCodexModelSupportsImageInput(modelID, accounts) {
			imageInputModels[modelID] = true
		}
		if groupCodexModelSupportsSearchTool(modelID, accounts) {
			searchToolModels[modelID] = true
		}
	}
	return buildCodexModelsManifest(modelIDs, imageInputModels, searchToolModels)
}

func buildCodexModelsManifest(modelIDs []string, imageInputModels, searchToolModels map[string]bool) ([]byte, error) {
	seen := make(map[string]struct{}, len(modelIDs))
	models := make([]json.RawMessage, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if _, exists := seen[modelID]; exists {
			continue
		}
		seen[modelID] = struct{}{}
		descriptor := newConfiguredCodexModelDescriptor(modelID)
		descriptor.SupportsSearchTool = searchToolModels[modelID]
		if imageInputModels[modelID] {
			descriptor.InputModalities = []string{"text", "image"}
		}
		raw, err := json.Marshal(descriptor)
		if err != nil {
			return nil, fmt.Errorf("encode Codex model %q: %w", modelID, err)
		}
		models = append(models, raw)
	}
	body, err := json.Marshal(map[string][]json.RawMessage{"models": models})
	if err != nil {
		return nil, fmt.Errorf("encode Codex models manifest: %w", err)
	}
	return body, nil
}

func groupCodexModelSupportsImageInput(modelID string, accounts []Account) bool {
	if len(accounts) == 0 {
		return accountCodexModelSupportsImageInput(nil, modelID)
	}
	matched := false
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformOpenAI {
			continue
		}
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			if accountCodexModelSupportsImageInput(account, modelID) {
				return true
			}
			matched = true
			continue
		}
		upstream, ok := mapping[modelID]
		if !ok {
			continue
		}
		matched = true
		if accountCodexModelSupportsImageInput(account, strings.TrimSpace(upstream)) {
			return true
		}
	}
	if !matched {
		return accountCodexModelSupportsImageInput(nil, modelID)
	}
	return false
}

func groupCodexModelSupportsSearchTool(modelID string, accounts []Account) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return false
	}
	candidates := 0
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != PlatformOpenAI {
			continue
		}
		mapping := account.GetModelMapping()
		if len(mapping) == 0 {
			if !account.IsModelSupported(modelID) {
				continue
			}
			candidates++
			if !shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
				return false
			}
			continue
		}
		if _, ok := mapping[modelID]; !ok {
			continue
		}
		candidates++
		if !shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
			return false
		}
	}
	return candidates > 0
}

func accountCodexModelSupportsImageInput(account *Account, upstreamModel string) bool {
	if !isOpenAICodexImageInputModel(upstreamModel) {
		return false
	}
	if account == nil {
		return true
	}
	switch account.Platform {
	case PlatformOpenAI:
		if !account.IsOpenAIApiKey() {
			return true
		}
		// Compatible model lists often omit modalities. Preserve the known GPT
		// fallback unless a synced snapshot later explicitly narrows it.
		return true
	default:
		return false
	}
}
