package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OmittedSettingKeys marks setting keys the caller's payload never carried.
// SystemSettings is a plain struct, so a field the caller omitted arrives as a
// zero value and is indistinguishable from a deliberate clear. Listing the key
// here drops it from the write, leaving the stored value in place.
//
// A nil or empty set keeps whole-document semantics: every key is written.
type OmittedSettingKeys map[string]struct{}

func (o OmittedSettingKeys) dropFrom(updates map[string]string) {
	for key := range o {
		delete(updates, key)
	}
}

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	return s.UpdateSettingsOmitting(ctx, settings, nil)
}

// UpdateSettingsOmitting persists system settings, leaving the keys in omitted
// at their stored value.
func (s *SettingService) UpdateSettingsOmitting(ctx context.Context, settings *SystemSettings, omitted OmittedSettingKeys) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
	}
	omitted.dropFrom(updates)

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return err
	}
	s.refreshCachedSettingsAfterWrite(ctx, settings, omitted)
	return nil
}

// UpdateSettingsWithAuthSourceDefaults persists system settings and auth-source defaults in a single write.
func (s *SettingService) UpdateSettingsWithAuthSourceDefaults(ctx context.Context, settings *SystemSettings, authDefaults *AuthSourceDefaultSettings) error {
	return s.UpdateSettingsWithAuthSourceDefaultsOmitting(ctx, settings, authDefaults, nil)
}

// UpdateSettingsWithAuthSourceDefaultsOmitting persists system settings and
// auth-source defaults in a single write, leaving the keys in omitted at their
// stored value.
func (s *SettingService) UpdateSettingsWithAuthSourceDefaultsOmitting(ctx context.Context, settings *SystemSettings, authDefaults *AuthSourceDefaultSettings, omitted OmittedSettingKeys) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
	}

	authSourceUpdates, err := s.buildAuthSourceDefaultUpdates(ctx, authDefaults)
	if err != nil {
		return err
	}
	for key, value := range authSourceUpdates {
		updates[key] = value
	}
	omitted.dropFrom(updates)

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return err
	}
	s.refreshCachedSettingsAfterWrite(ctx, settings, omitted)
	return nil
}

// refreshCachedSettingsAfterWrite keeps the in-process caches in step with the
// write that just landed. A partial payload carries zero values for the fields
// it omitted, so in that case the caches are rebuilt from storage rather than
// from the request struct.
func (s *SettingService) refreshCachedSettingsAfterWrite(ctx context.Context, settings *SystemSettings, omitted OmittedSettingKeys) {
	if len(omitted) == 0 {
		s.refreshCachedSettings(settings)
		return
	}
	stored, err := s.GetAllSettings(ctx)
	if err != nil {
		slog.Warn("refresh cached settings after partial update failed", "error", err)
		return
	}
	s.refreshCachedSettings(stored)
}

func (s *SettingService) buildSystemSettingsUpdates(ctx context.Context, settings *SystemSettings) (map[string]string, error) {
	if err := s.validateDefaultSubscriptionGroups(ctx, settings.DefaultSubscriptions); err != nil {
		return nil, err
	}
	normalizedWhitelist, err := NormalizeRegistrationEmailSuffixWhitelist(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", err.Error())
	}
	if normalizedWhitelist == nil {
		normalizedWhitelist = []string{}
	}
	settings.RegistrationEmailSuffixWhitelist = normalizedWhitelist
	normalizedForwardedClientIPHeaders, err := config.NormalizeForwardedClientIPHeaders(settings.ForwardedClientIPHeaders)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_FORWARDED_CLIENT_IP_HEADERS", err.Error())
	}
	settings.ForwardedClientIPHeaders = normalizedForwardedClientIPHeaders
	if err := s.normalizeOpenAIAdvancedSchedulerOverrides(settings); err != nil {
		return nil, err
	}

	updates := make(map[string]string)

	// 注册邮箱策略设置
	registrationEmailSuffixWhitelistJSON, err := json.Marshal(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return nil, fmt.Errorf("marshal registration email suffix whitelist: %w", err)
	}
	updates[SettingKeyRegistrationEmailSuffixWhitelist] = string(registrationEmailSuffixWhitelistJSON)
	updates[SettingKeyRegistrationEmailDomainQuotaEnabled] = strconv.FormatBool(settings.RegistrationEmailDomainQuotaEnabled)
	updates[SettingKeyTotpEnabled] = strconv.FormatBool(settings.TotpEnabled)
	updates[SettingKeyPasskeyEnabled] = strconv.FormatBool(settings.PasskeyEnabled)
	updates[SettingKeySessionBindingEnabled] = strconv.FormatBool(settings.SessionBindingEnabled)

	// 登录入口 / 默认首页。handler 已经用 NormalizeAndValidateWebEntryInput 挡过一次，
	// 这里再归一化一次，保证直接调用 service 的路径也存不进 "/gate/" 这种未归一化的值。
	updates[SettingKeyWebLoginEntryPublic] = strconv.FormatBool(settings.LoginEntryPublic)
	updates[SettingKeyWebLoginEntryPath] = config.NormalizeEntryPath(settings.LoginEntryPath)
	updates[SettingKeyWebDefaultHomePath] = config.NormalizeEntryPath(settings.DefaultHomePath)
	updates[SettingKeyStepUpEnabled] = strconv.FormatBool(settings.StepUpEnabled)
	updates[SettingKeyAuditLogRetentionDays] = strconv.Itoa(settings.AuditLogRetentionDays)
	updates[SettingKeyAPIKeyACLTrustForwardedIP] = strconv.FormatBool(settings.APIKeyACLTrustForwardedIP)
	forwardedClientIPHeadersJSON, err := json.Marshal(settings.ForwardedClientIPHeaders)
	if err != nil {
		return nil, fmt.Errorf("marshal forwarded client IP headers: %w", err)
	}
	updates[SettingKeyForwardedClientIPHeaders] = string(forwardedClientIPHeadersJSON)

	// OEM设置
	updates[SettingKeyDocURL] = settings.DocURL
	updates[SettingKeyCustomEndpoints] = settings.CustomEndpoints

	// 默认配置
	updates[SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)
	updates[SettingKeyDefaultUserRPMLimit] = strconv.Itoa(settings.DefaultUserRPMLimit)
	defaultSubsJSON, err := json.Marshal(settings.DefaultSubscriptions)
	if err != nil {
		return nil, fmt.Errorf("marshal default subscriptions: %w", err)
	}
	updates[SettingKeyDefaultSubscriptions] = string(defaultSubsJSON)

	// Model fallback configuration
	updates[SettingKeyEnableModelFallback] = strconv.FormatBool(settings.EnableModelFallback)
	updates[SettingKeyFallbackModelAnthropic] = settings.FallbackModelAnthropic
	updates[SettingKeyFallbackModelOpenAI] = settings.FallbackModelOpenAI
	updates[SettingKeyFallbackModelGemini] = settings.FallbackModelGemini
	updates[SettingKeyFallbackModelAntigravity] = settings.FallbackModelAntigravity

	// Identity patch configuration (Claude -> Gemini)
	updates[SettingKeyEnableIdentityPatch] = strconv.FormatBool(settings.EnableIdentityPatch)
	updates[SettingKeyIdentityPatchPrompt] = settings.IdentityPatchPrompt

	// Ops monitoring (vNext)
	updates[SettingKeyOpsMonitoringEnabled] = strconv.FormatBool(settings.OpsMonitoringEnabled)
	updates[SettingKeyOpsRealtimeMonitoringEnabled] = strconv.FormatBool(settings.OpsRealtimeMonitoringEnabled)
	updates[SettingKeyOpsQueryModeDefault] = string(ParseOpsQueryMode(settings.OpsQueryModeDefault))
	if settings.OpsMetricsIntervalSeconds > 0 {
		updates[SettingKeyOpsMetricsIntervalSeconds] = strconv.Itoa(settings.OpsMetricsIntervalSeconds)
	}

	// Channel monitor feature switch
	updates[SettingKeyChannelMonitorEnabled] = strconv.FormatBool(settings.ChannelMonitorEnabled)
	updates[SettingKeyChannelMonitorHideThroughput] = strconv.FormatBool(settings.ChannelMonitorHideThroughput)

	// Grok model mapping policy
	if v := strings.TrimSpace(settings.GrokDefaultTextModel); v != "" {
		updates[SettingKeyGrokDefaultTextModel] = v
	} else {
		updates[SettingKeyGrokDefaultTextModel] = "grok-4.6"
	}
	updates[SettingKeyGrokCrossClientModelMapEnabled] = strconv.FormatBool(settings.GrokCrossClientModelMapEnabled)
	updates[SettingKeyGrokDefaultBaseURLMode] = normalizeGrokDefaultBaseURLMode(settings.GrokDefaultBaseURLMode)

	// Available channels feature switch
	updates[SettingKeyAvailableChannelsEnabled] = strconv.FormatBool(settings.AvailableChannelsEnabled)


	// 风控中心功能开关
	updates[SettingKeyRiskControlEnabled] = strconv.FormatBool(settings.RiskControlEnabled)

	// cyber 会话屏蔽开关 + TTL
	updates[SettingKeyCyberSessionBlockEnabled] = strconv.FormatBool(settings.CyberSessionBlockEnabled)
	if settings.CyberSessionBlockTTLSeconds > 0 {
		updates[SettingKeyCyberSessionBlockTTLSeconds] = strconv.Itoa(settings.CyberSessionBlockTTLSeconds)
	}

	// Claude Code version check
	updates[SettingKeyMinClaudeCodeVersion] = settings.MinClaudeCodeVersion
	updates[SettingKeyMaxClaudeCodeVersion] = settings.MaxClaudeCodeVersion

	// 分组隔离
	updates[SettingKeyAllowUngroupedKeyScheduling] = strconv.FormatBool(settings.AllowUngroupedKeyScheduling)

	// Backend Mode
	updates[SettingKeyBackendModeEnabled] = strconv.FormatBool(settings.BackendModeEnabled)

	// Gateway forwarding behavior
	updates[SettingKeyEnableFingerprintUnification] = strconv.FormatBool(settings.EnableFingerprintUnification)
	updates[SettingKeyEnableMetadataPassthrough] = strconv.FormatBool(settings.EnableMetadataPassthrough)
	updates[SettingKeyEnableCCHSigning] = strconv.FormatBool(settings.EnableCCHSigning)
	updates[SettingKeyEnableClaudeOAuthSystemPromptInjection] = strconv.FormatBool(settings.EnableClaudeOAuthSystemPromptInjection)
	updates[SettingKeyClaudeOAuthSystemPrompt] = settings.ClaudeOAuthSystemPrompt
	if err := ValidateClaudeOAuthSystemPromptBlocksConfig(settings.ClaudeOAuthSystemPromptBlocks); err != nil {
		return nil, err
	}
	updates[SettingKeyClaudeOAuthSystemPromptBlocks] = settings.ClaudeOAuthSystemPromptBlocks
	updates[SettingKeyEnableAnthropicCacheTTL1hInjection] = strconv.FormatBool(settings.EnableAnthropicCacheTTL1hInjection)
	updates[SettingKeyRewriteMessageCacheControl] = strconv.FormatBool(settings.RewriteMessageCacheControl)
	updates[SettingKeyEnableClientDatelineNormalization] = strconv.FormatBool(settings.EnableClientDatelineNormalization)
	updates[SettingKeyAntigravityUserAgentVersion] = antigravity.NormalizeUserAgentVersion(settings.AntigravityUserAgentVersion)
	updates[SettingKeyOpenAICodexUserAgent] = strings.TrimSpace(settings.OpenAICodexUserAgent)
	updates[SettingKeyOpenAICodexClientVersion] = NormalizeCodexClientVersion(settings.OpenAICodexClientVersion)
	updates[SettingKeyOpenAICodexVersionAutoSyncEnabled] = strconv.FormatBool(settings.OpenAICodexVersionAutoSyncEnabled)
	// SettingKeyOpenAICodexClientVersionSynced 由自动同步任务独占写入，此处不得覆盖，
	// 否则面板保存会把同步结果清空。
	// codex_cli_only 加固
	updates[SettingKeyMinCodexVersion] = strings.TrimSpace(settings.MinCodexVersion)
	updates[SettingKeyMaxCodexVersion] = strings.TrimSpace(settings.MaxCodexVersion)
	updates[SettingKeyCodexCLIOnlyBlacklist] = strings.TrimSpace(settings.CodexCLIOnlyBlacklist)
	updates[SettingKeyCodexCLIOnlyWhitelist] = strings.TrimSpace(settings.CodexCLIOnlyWhitelist)
	updates[SettingKeyCodexCLIOnlyAllowAppServerClients] = strconv.FormatBool(settings.CodexCLIOnlyAllowAppServerClients)
	updates[SettingKeyCodexCLIOnlyEngineFingerprintSignals] = strings.TrimSpace(settings.CodexCLIOnlyEngineFingerprintSignals)
	updates[SettingKeyOpenAILowUpstreamRatePriorityEnabled] = strconv.FormatBool(settings.OpenAILowUpstreamRatePriorityEnabled)
	updates[SettingKeyOpenAIOAuthSchedulingRateMultiplier] = strconv.FormatFloat(settings.OpenAIOAuthSchedulingRateMultiplier, 'f', -1, 64)
	updates[openAIAdvancedSchedulerSettingKey] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerStickyWeightedEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerLBTopK] = settings.OpenAIAdvancedSchedulerLBTopK
	updates[SettingKeyOpenAIAdvancedSchedulerWeightPriority] = settings.OpenAIAdvancedSchedulerWeightPriority
	updates[SettingKeyOpenAIAdvancedSchedulerWeightLoad] = settings.OpenAIAdvancedSchedulerWeightLoad
	updates[SettingKeyOpenAIAdvancedSchedulerWeightQueue] = settings.OpenAIAdvancedSchedulerWeightQueue
	updates[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate] = settings.OpenAIAdvancedSchedulerWeightErrorRate
	updates[SettingKeyOpenAIAdvancedSchedulerWeightTTFT] = settings.OpenAIAdvancedSchedulerWeightTTFT
	updates[SettingKeyOpenAIAdvancedSchedulerWeightReset] = settings.OpenAIAdvancedSchedulerWeightReset
	updates[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom] = settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom
	updates[SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost] = settings.OpenAIAdvancedSchedulerWeightUpstreamCost
	updates[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse] = settings.OpenAIAdvancedSchedulerWeightPreviousResponse
	updates[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky] = settings.OpenAIAdvancedSchedulerWeightSessionSticky

	// 系统全局 platform quota：整体替换语义（null/缺省 = 不限制）。
	if settings.DefaultPlatformQuotas != nil {
		if err := validateDefaultPlatformQuotaMap(settings.DefaultPlatformQuotas); err != nil {
			return nil, err
		}
		blob, err := json.Marshal(settings.DefaultPlatformQuotas)
		if err != nil {
			return nil, fmt.Errorf("marshal default platform quotas: %w", err)
		}
		updates[SettingKeyDefaultPlatformQuotas] = string(blob)
	}
	if settings.AccountSchedulingThresholds != nil {
		normalized, err := validateAndNormalizeAccountSchedulingThresholds(settings.AccountSchedulingThresholds)
		if err != nil {
			return nil, err
		}
		blob, err := json.Marshal(normalized)
		if err != nil {
			return nil, fmt.Errorf("marshal account scheduling thresholds: %w", err)
		}
		updates[SettingKeyAccountSchedulingThresholds] = string(blob)
	}

	updates[SettingKeyAllowUserViewErrorRequests] = strconv.FormatBool(settings.AllowUserViewErrorRequests)

	return updates, nil
}

func defaultAccountSchedulingThresholds() map[string]int {
	return map[string]int{
		PlatformOpenAI:    100,
		PlatformAnthropic: 100,
		PlatformGrok:      100,
	}
}

func validateAndNormalizeAccountSchedulingThresholds(input map[string]int) (map[string]int, error) {
	normalized := defaultAccountSchedulingThresholds()
	for platform, value := range input {
		allowed := false
		for _, item := range AllowedSchedulingThresholdPlatforms {
			if item == platform {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, infraerrors.BadRequest("INVALID_ACCOUNT_SCHEDULING_THRESHOLDS", fmt.Sprintf("unknown platform %q", platform))
		}
		if value < 1 || value > 100 {
			return nil, infraerrors.BadRequest("INVALID_ACCOUNT_SCHEDULING_THRESHOLDS", "platform scheduling threshold must be between 1 and 100")
		}
		normalized[platform] = value
	}
	return normalized, nil
}

func parseAccountSchedulingThresholdsSetting(raw string) (map[string]int, error) {
	thresholds := defaultAccountSchedulingThresholds()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return thresholds, nil
	}
	parsed := map[string]int{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return thresholds, err
	}
	for _, platform := range AllowedSchedulingThresholdPlatforms {
		if value, ok := parsed[platform]; ok {
			thresholds[platform] = boundedIntOrDefault(value, 1, 100, 100)
		}
	}
	return thresholds, nil
}

func boundedIntOrDefault(value, minValue, maxValue, defaultValue int) int {
	if value < minValue || value > maxValue {
		return defaultValue
	}
	return value
}

func cloneAccountSchedulingThresholds(input map[string]int) map[string]int {
	if len(input) == 0 {
		return defaultAccountSchedulingThresholds()
	}
	cloned := make(map[string]int, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

// validateDefaultPlatformQuotaMap 校验 platform quota map 的合法性：
// 平台名须在 AllowedQuotaPlatforms 白名单内，每个非 nil 上限须 finite 且 >= 0。
// 系统层和 auth-source 层共用此 helper。
func validateDefaultPlatformQuotaMap(m map[string]*DefaultPlatformQuotaSetting) error {
	for platform, pq := range m {
		if !IsAllowedQuotaPlatform(platform) {
			return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", fmt.Sprintf("unknown platform %q", platform))
		}
		if pq == nil {
			continue
		}
		for _, v := range []*float64{pq.DailyLimitUSD, pq.WeeklyLimitUSD, pq.MonthlyLimitUSD} {
			if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
				return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", "platform quota limit must be a finite non-negative number")
			}
		}
	}
	return nil
}

func (s *SettingService) buildAuthSourceDefaultUpdates(ctx context.Context, settings *AuthSourceDefaultSettings) (map[string]string, error) {
	if settings == nil {
		return nil, nil
	}

	if err := s.validateDefaultSubscriptionGroups(ctx, settings.Email.Subscriptions); err != nil {
		return nil, err
	}

	// 校验 email auth source 的 platform quota map（对等系统层校验）
	if settings.Email.PlatformQuotas != nil {
		if err := validateDefaultPlatformQuotaMap(settings.Email.PlatformQuotas); err != nil {
			return nil, err
		}
	}

	updates := make(map[string]string, 6)
	writeProviderDefaultGrantUpdates(updates, emailAuthSourceDefaultKeys, settings.Email)
	return updates, nil
}

func (s *SettingService) refreshCachedSettings(settings *SystemSettings) {
	if settings == nil {
		return
	}

	// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口
	versionBoundsSF.Forget("version_bounds")
	versionBoundsCache.Store(&cachedVersionBounds{
		min:       settings.MinClaudeCodeVersion,
		max:       settings.MaxClaudeCodeVersion,
		expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
	})
	backendModeSF.Forget("backend_mode")
	backendModeCache.Store(&cachedBackendMode{
		value:     settings.BackendModeEnabled,
		expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
	})
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification:           settings.EnableFingerprintUnification,
		metadataPassthrough:              settings.EnableMetadataPassthrough,
		cchSigning:                       settings.EnableCCHSigning,
		claudeOAuthSystemPromptInjection: settings.EnableClaudeOAuthSystemPromptInjection,
		claudeOAuthSystemPrompt:          settings.ClaudeOAuthSystemPrompt,
		claudeOAuthSystemPromptBlocks:    settings.ClaudeOAuthSystemPromptBlocks,
		anthropicCacheTTL1hInjection:     settings.EnableAnthropicCacheTTL1hInjection,
		rewriteMessageCacheControl:       settings.RewriteMessageCacheControl,
		clientDatelineNormalization:      settings.EnableClientDatelineNormalization,
		expiresAt:                        time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
	})
	s.antigravityUAVersionSF.Forget("antigravity_user_agent_version")
	antigravityUserAgentVersion := antigravity.NormalizeUserAgentVersion(settings.AntigravityUserAgentVersion)
	if antigravityUserAgentVersion == "" {
		antigravityUserAgentVersion = antigravity.GetDefaultUserAgentVersion()
	}
	s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
		version:   antigravityUserAgentVersion,
		expiresAt: time.Now().Add(antigravityUserAgentVersionCacheTTL).UnixNano(),
	})
	s.openAICodexUASF.Forget("openai_codex_user_agent")
	codexUA := strings.TrimSpace(settings.OpenAICodexUserAgent)
	if codexUA == "" {
		codexUA = DefaultOpenAICodexUserAgent
	}
	s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
		value:     codexUA,
		expiresAt: time.Now().Add(openAICodexUserAgentCacheTTL).UnixNano(),
	})
	// 版本号缓存只做失效，不在此重算：生效值还取决于自动同步写入的 synced 键，
	// 这里没有它的最新值，重算会把同步结果覆盖成陈旧值。
	s.InvalidateOpenAICodexClientVersionCache()
	openAIAdvancedSchedulerSettingSF.Forget(openAIAdvancedSchedulerSettingKey)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		lowUpstreamRatePriorityEnabled: settings.OpenAILowUpstreamRatePriorityEnabled,
		oauthSchedulingRateMultiplier:  settings.OpenAIOAuthSchedulingRateMultiplier,
		enabled:                        settings.OpenAIAdvancedSchedulerEnabled,
		stickyWeightedEnabled:          settings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		subscriptionPriorityEnabled:    settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		lbTopKOverride:                 parsePositiveIntOverride(settings.OpenAIAdvancedSchedulerLBTopK),
		weightOverrides: parseOpenAIAdvancedSchedulerWeightOverrides(map[string]string{
			SettingKeyOpenAIAdvancedSchedulerWeightPriority:         settings.OpenAIAdvancedSchedulerWeightPriority,
			SettingKeyOpenAIAdvancedSchedulerWeightLoad:             settings.OpenAIAdvancedSchedulerWeightLoad,
			SettingKeyOpenAIAdvancedSchedulerWeightQueue:            settings.OpenAIAdvancedSchedulerWeightQueue,
			SettingKeyOpenAIAdvancedSchedulerWeightErrorRate:        settings.OpenAIAdvancedSchedulerWeightErrorRate,
			SettingKeyOpenAIAdvancedSchedulerWeightTTFT:             settings.OpenAIAdvancedSchedulerWeightTTFT,
			SettingKeyOpenAIAdvancedSchedulerWeightReset:            settings.OpenAIAdvancedSchedulerWeightReset,
			SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom:    settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
			SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost:     settings.OpenAIAdvancedSchedulerWeightUpstreamCost,
			SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse: settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
			SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky:    settings.OpenAIAdvancedSchedulerWeightSessionSticky,
		}),
		expiresAt: time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
	})
	// Invalidate the quota auto-pause cache and let the next read trigger a fresh load.
	// We can't know from here whether ops_advanced_settings was also touched, so be
	// defensive: store an expired entry — GetOpenAIQuotaAutoPauseSettings will serve
	// stale and kick off an async refresh, never blocking the request that follows.
	s.openAIQuotaAutoPauseSettingsSF.Forget(openAIQuotaAutoPauseSettingsRefreshKey)
	if cached, _ := s.openAIQuotaAutoPauseSettingsCache.Load().(*cachedOpenAIQuotaAutoPauseSettings); cached != nil {
		s.openAIQuotaAutoPauseSettingsCache.Store(&cachedOpenAIQuotaAutoPauseSettings{
			settings:  cached.settings,
			expiresAt: 0,
		})
	}
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	if settings.AccountSchedulingThresholds != nil {
		normalizedThresholds, err := validateAndNormalizeAccountSchedulingThresholds(settings.AccountSchedulingThresholds)
		if err != nil {
			normalizedThresholds = defaultAccountSchedulingThresholds()
		}
		accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{
			thresholds: cloneAccountSchedulingThresholds(normalizedThresholds),
			expiresAt:  time.Now().Add(accountSchedulingThresholdsCacheTTL).UnixNano(),
		})
	} else {
		// Partial/omitted payload: clear cache so the next hot-path read reloads from DB.
		accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})
	}
	if s.cfg != nil {
		s.cfg.SetForwardedClientIPSettings(settings.APIKeyACLTrustForwardedIP, settings.ForwardedClientIPHeaders)
	}
	// codex_cli_only 加固策略缓存：设置更新后强制下次重载（涉及 4 个键 + JSON 解析，直接置过期）。
	s.codexRestrictionPolicySF.Forget("codex_restriction_policy")
	s.codexRestrictionPolicyCache.Store(&cachedCodexRestrictionPolicy{expiresAt: 0})
	// 登录入口 / 默认首页：直接用刚写下去的这份值重建缓存（不回读数据库），
	// 本节点下一个请求即生效。必须排在 onUpdate 之前——onUpdate 会让内嵌前端的
	// HTML 缓存失效，重新渲染时要读到新的登录入口状态（决定是否注入登录标记）。
	s.refreshWebEntryCacheFromSettings(settings)
	if s.onUpdate != nil {
		s.onUpdate() // Invalidate cache after settings update
	}
	s.notifyChannelMonitorRuntimeListeners()
}

func (s *SettingService) defaultRewriteMessageCacheControl() bool {
	return false
}

func (s *SettingService) validateDefaultSubscriptionGroups(ctx context.Context, items []DefaultSubscriptionSetting) error {
	if len(items) == 0 {
		return nil
	}

	checked := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.GroupID <= 0 {
			continue
		}
		if _, ok := checked[item.GroupID]; ok {
			return ErrDefaultSubGroupDuplicate.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
		checked[item.GroupID] = struct{}{}
		if s.defaultSubGroupReader == nil {
			continue
		}

		group, err := s.defaultSubGroupReader.GetByID(ctx, item.GroupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(item.GroupID, 10),
				})
			}
			return fmt.Errorf("get default subscription group %d: %w", item.GroupID, err)
		}
		if !group.IsSubscriptionType() {
			return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
	}

	return nil
}
