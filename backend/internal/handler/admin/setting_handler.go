package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// semverPattern 预编译 semver 格式校验正则
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// menuItemIDPattern validates custom menu item IDs: alphanumeric, hyphens, underscores only.
var menuItemIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// generateMenuItemID generates a short random hex ID for a custom menu item.
func generateMenuItemID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate menu item ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService           *service.SettingService
	emailService             *service.EmailService
	opsService               *service.OpsService
	notificationEmailService *service.NotificationEmailService
	totpService              *service.TotpService
	userService              *service.UserService
}

// NewSettingHandler 创建系统设置处理器。
// 第五个参数（原钉钉身份同步的 UserAttributeService）随 OAuth 登录裁剪不再使用，
// 保留形参以免改动 wire 装配与既有测试的构造签名。
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService, opsService *service.OpsService, _ *service.UserAttributeService) *SettingHandler {
	return &SettingHandler{
		settingService: settingService,
		emailService:   emailService,
		opsService:     opsService,
	}
}

// SetNotificationEmailService attaches the notification template service without changing
// the constructor signature used by existing unit tests.
func (h *SettingHandler) SetNotificationEmailService(notificationEmailService *service.NotificationEmailService) {
	h.notificationEmailService = notificationEmailService
}

// SetStepUpDeps attaches the services backing the step-up switch preconditions
// (enable requires the acting admin to have TOTP enabled; disable is itself a
// step-up gated operation), without changing the constructor signature used by
// existing unit tests.
func (h *SettingHandler) SetStepUpDeps(totpService *service.TotpService, userService *service.UserService) {
	h.totpService = totpService
	h.userService = userService
}

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	authSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Check if ops monitoring is enabled (respects config.ops.enabled)
	opsEnabled := h.opsService != nil && h.opsService.IsMonitoringEnabled(c.Request.Context())
	defaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(settings.DefaultSubscriptions))
	for _, sub := range settings.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	passkeyConfigured, passkeyRPID, passkeyRPOrigins := h.settingService.PasskeyConfiguration()

	payload := dto.SystemSettings{
		EmailVerifyEnabled:                           settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:             settings.RegistrationEmailSuffixWhitelist,
		RegistrationEmailDomainQuotaEnabled:          settings.RegistrationEmailDomainQuotaEnabled,
		PasswordResetEnabled:                         settings.PasswordResetEnabled,
		FrontendURL:                                  settings.FrontendURL,
		TotpEnabled:                                  settings.TotpEnabled,
		TotpEncryptionKeyConfigured:                  h.settingService.IsTotpEncryptionKeyConfigured(),
		PasskeyEnabled:                               settings.PasskeyEnabled,
		PasskeyConfigured:                            passkeyConfigured,
		PasskeyRPID:                                  passkeyRPID,
		PasskeyRPOrigins:                             passkeyRPOrigins,
		SessionBindingEnabled:                        settings.SessionBindingEnabled,
		StepUpEnabled:                                settings.StepUpEnabled,
		AuditLogRetentionDays:                        settings.AuditLogRetentionDays,
		LoginAgreementEnabled:                        settings.LoginAgreementEnabled,
		LoginAgreementMode:                           settings.LoginAgreementMode,
		LoginAgreementUpdatedAt:                      settings.LoginAgreementUpdatedAt,
		LoginAgreementDocuments:                      loginAgreementDocumentsToDTO(settings.LoginAgreementDocuments),
		SMTPHost:                                     settings.SMTPHost,
		SMTPPort:                                     settings.SMTPPort,
		SMTPUsername:                                 settings.SMTPUsername,
		SMTPPasswordConfigured:                       settings.SMTPPasswordConfigured,
		SMTPFrom:                                     settings.SMTPFrom,
		SMTPFromName:                                 settings.SMTPFromName,
		SMTPUseTLS:                                   settings.SMTPUseTLS,
		APIKeyACLTrustForwardedIP:                    settings.APIKeyACLTrustForwardedIP,
		ForwardedClientIPHeaders:                     settings.ForwardedClientIPHeaders,
		SiteName:                                     settings.SiteName,
		SiteLogo:                                     settings.SiteLogo,
		SiteSubtitle:                                 settings.SiteSubtitle,
		APIBaseURL:                                   settings.APIBaseURL,
		ContactInfo:                                  settings.ContactInfo,
		DocURL:                                       settings.DocURL,
		HomeContent:                                  settings.HomeContent,
		CompactHomeEnabled:                           settings.CompactHomeEnabled,
		HideCcsImportButton:                          settings.HideCcsImportButton,
		TableDefaultPageSize:                         settings.TableDefaultPageSize,
		TablePageSizeOptions:                         settings.TablePageSizeOptions,
		CustomMenuItems:                              dto.ParseCustomMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                              dto.ParseCustomEndpoints(settings.CustomEndpoints),
		DefaultConcurrency:                           settings.DefaultConcurrency,
		DefaultBalance:                               settings.DefaultBalance,
		RiskControlEnabled:                           settings.RiskControlEnabled,
		CyberSessionBlockEnabled:                     settings.CyberSessionBlockEnabled,
		CyberSessionBlockTTLSeconds:                  settings.CyberSessionBlockTTLSeconds,
		DefaultUserRPMLimit:                          settings.DefaultUserRPMLimit,
		DefaultSubscriptions:                         defaultSubscriptions,
		EnableModelFallback:                          settings.EnableModelFallback,
		FallbackModelAnthropic:                       settings.FallbackModelAnthropic,
		FallbackModelOpenAI:                          settings.FallbackModelOpenAI,
		FallbackModelGemini:                          settings.FallbackModelGemini,
		FallbackModelAntigravity:                     settings.FallbackModelAntigravity,
		EnableIdentityPatch:                          settings.EnableIdentityPatch,
		IdentityPatchPrompt:                          settings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                         opsEnabled && settings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:                 settings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                          settings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:                    settings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                         settings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                         settings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:                  settings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                           settings.BackendModeEnabled,
		EnableFingerprintUnification:                 settings.EnableFingerprintUnification,
		EnableMetadataPassthrough:                    settings.EnableMetadataPassthrough,
		EnableCCHSigning:                             settings.EnableCCHSigning,
		EnableClaudeOAuthSystemPromptInjection:       settings.EnableClaudeOAuthSystemPromptInjection,
		ClaudeOAuthSystemPrompt:                      settings.ClaudeOAuthSystemPrompt,
		ClaudeOAuthSystemPromptBlocks:                settings.ClaudeOAuthSystemPromptBlocks,
		EnableAnthropicCacheTTL1hInjection:           settings.EnableAnthropicCacheTTL1hInjection,
		RewriteMessageCacheControl:                   settings.RewriteMessageCacheControl,
		EnableClientDatelineNormalization:            settings.EnableClientDatelineNormalization,
		AntigravityUserAgentVersion:                  settings.AntigravityUserAgentVersion,
		OpenAICodexUserAgent:                         settings.OpenAICodexUserAgent,
		OpenAICodexClientVersion:                     settings.OpenAICodexClientVersion,
		OpenAICodexClientVersionSynced:               settings.OpenAICodexClientVersionSynced,
		OpenAICodexVersionAutoSyncEnabled:            settings.OpenAICodexVersionAutoSyncEnabled,
		MinCodexVersion:                              settings.MinCodexVersion,
		MaxCodexVersion:                              settings.MaxCodexVersion,
		CodexCLIOnlyBlacklist:                        settings.CodexCLIOnlyBlacklist,
		CodexCLIOnlyWhitelist:                        settings.CodexCLIOnlyWhitelist,
		CodexCLIOnlyAllowAppServerClients:            settings.CodexCLIOnlyAllowAppServerClients,
		CodexCLIOnlyEngineFingerprintSignals:         settings.CodexCLIOnlyEngineFingerprintSignals,
		WebSearchEmulationEnabled:                    settings.WebSearchEmulationEnabled,
		OpenAILowUpstreamRatePriorityEnabled:         settings.OpenAILowUpstreamRatePriorityEnabled,
		OpenAIOAuthSchedulingRateMultiplier:          settings.OpenAIOAuthSchedulingRateMultiplier,
		OpenAIAdvancedSchedulerEnabled:               settings.OpenAIAdvancedSchedulerEnabled,
		OpenAIAdvancedSchedulerStickyWeightedEnabled: settings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled:     settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		OpenAIAdvancedSchedulerLBTopK:                          settings.OpenAIAdvancedSchedulerLBTopK,
		OpenAIAdvancedSchedulerWeightPriority:                  settings.OpenAIAdvancedSchedulerWeightPriority,
		OpenAIAdvancedSchedulerWeightLoad:                      settings.OpenAIAdvancedSchedulerWeightLoad,
		OpenAIAdvancedSchedulerWeightQueue:                     settings.OpenAIAdvancedSchedulerWeightQueue,
		OpenAIAdvancedSchedulerWeightErrorRate:                 settings.OpenAIAdvancedSchedulerWeightErrorRate,
		OpenAIAdvancedSchedulerWeightTTFT:                      settings.OpenAIAdvancedSchedulerWeightTTFT,
		OpenAIAdvancedSchedulerWeightReset:                     settings.OpenAIAdvancedSchedulerWeightReset,
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:             settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerWeightUpstreamCost:              settings.OpenAIAdvancedSchedulerWeightUpstreamCost,
		OpenAIAdvancedSchedulerWeightPreviousResponse:          settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		OpenAIAdvancedSchedulerWeightSessionSticky:             settings.OpenAIAdvancedSchedulerWeightSessionSticky,
		OpenAIAdvancedSchedulerEffectiveLBTopK:                 settings.OpenAIAdvancedSchedulerEffectiveLBTopK,
		OpenAIAdvancedSchedulerEffectiveWeightPriority:         settings.OpenAIAdvancedSchedulerEffectiveWeightPriority,
		OpenAIAdvancedSchedulerEffectiveWeightLoad:             settings.OpenAIAdvancedSchedulerEffectiveWeightLoad,
		OpenAIAdvancedSchedulerEffectiveWeightQueue:            settings.OpenAIAdvancedSchedulerEffectiveWeightQueue,
		OpenAIAdvancedSchedulerEffectiveWeightErrorRate:        settings.OpenAIAdvancedSchedulerEffectiveWeightErrorRate,
		OpenAIAdvancedSchedulerEffectiveWeightTTFT:             settings.OpenAIAdvancedSchedulerEffectiveWeightTTFT,
		OpenAIAdvancedSchedulerEffectiveWeightReset:            settings.OpenAIAdvancedSchedulerEffectiveWeightReset,
		OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom:    settings.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost:     settings.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost,
		OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse: settings.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse,
		OpenAIAdvancedSchedulerEffectiveWeightSessionSticky:    settings.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky,
		BalanceLowNotifyEnabled:                                settings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:                              settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:                            settings.BalanceLowNotifyRechargeURL,
		SubscriptionExpiryNotifyEnabled:                        settings.SubscriptionExpiryNotifyEnabled,
		AccountQuotaNotifyEnabled:                              settings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:                               dto.NotifyEmailEntriesFromService(settings.AccountQuotaNotifyEmails),

		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorMode:                   settings.ChannelMonitorMode,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,
		ChannelMonitorHideThroughput:         settings.ChannelMonitorHideThroughput,
		ChannelMonitorShowQuota:              settings.ChannelMonitorShowQuota,

		GrokDefaultTextModel:           settings.GrokDefaultTextModel,
		GrokCrossClientModelMapEnabled: settings.GrokCrossClientModelMapEnabled,
		GrokDefaultBaseURLMode:         settings.GrokDefaultBaseURLMode,

		AvailableChannelsEnabled: settings.AvailableChannelsEnabled,

		ModelPlazaEnabled:     settings.ModelPlazaEnabled,
		ModelPlazaRequireAuth: settings.ModelPlazaRequireAuth,
		ModelPlazaDescription: settings.ModelPlazaDescription,

		AccountSchedulingThresholds: settings.AccountSchedulingThresholds,
		AllowUserViewErrorRequests:  settings.AllowUserViewErrorRequests,
	}

	// OpenAI fast policy (stored under a dedicated setting key)
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
	} else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
	}

	// Default platform quotas（JSON map）
	if platformQuotas, err := h.settingService.GetDefaultPlatformQuotas(c.Request.Context()); err != nil {
		slog.Error("default_platform_quotas_get_failed", "error", err)
	} else {
		payload.DefaultPlatformQuotas = platformQuotas
	}

	// 登录入口 / 默认首页：回生效值（本地配置 > 数据库 > 默认）而不是数据库原始值。
	// 界面上要显示"现在登录页到底在哪"，还要显示这一项是不是被配置文件锁住了。
	webEntrySettingsToDTO(h.settingService.ResolveWebEntry(c.Request.Context()), &payload)

	response.Success(c, systemSettingsResponseData(payload, authSourceDefaults))
}

// openaiFastPolicySettingsToDTO converts service -> dto for OpenAI fast policy.
func openaiFastPolicySettingsToDTO(s *service.OpenAIFastPolicySettings) *dto.OpenAIFastPolicySettings {
	if s == nil {
		return nil
	}
	rules := make([]dto.OpenAIFastPolicyRule, len(s.Rules))
	for i, r := range s.Rules {
		rules[i] = dto.OpenAIFastPolicyRule(r)
	}
	return &dto.OpenAIFastPolicySettings{Rules: rules}
}

// openaiFastPolicySettingsFromDTO converts dto -> service for OpenAI fast policy.
//
// 规范化 ServiceTier：在 DTO 进入 service 层之前统一把空字符串归一为
// service.OpenAIFastTierAny ("all")，避免管理员保存时空串与 "all" 同时
// 表达"匹配任意 tier"造成数据库取值的二义性。其它非空值原样透传，由
// service.SetOpenAIFastPolicySettings 负责合法值校验。
func openaiFastPolicySettingsFromDTO(s *dto.OpenAIFastPolicySettings) *service.OpenAIFastPolicySettings {
	if s == nil {
		return nil
	}
	rules := make([]service.OpenAIFastPolicyRule, len(s.Rules))
	for i, r := range s.Rules {
		rules[i] = service.OpenAIFastPolicyRule(r)
		tier := strings.ToLower(strings.TrimSpace(rules[i].ServiceTier))
		if tier == "" {
			tier = service.OpenAIFastTierAny
		}
		rules[i].ServiceTier = tier
	}
	return &service.OpenAIFastPolicySettings{Rules: rules}
}

func loginAgreementDocumentsToDTO(items []service.LoginAgreementDocument) []dto.LoginAgreementDocument {
	result := make([]dto.LoginAgreementDocument, 0, len(items))
	for _, item := range items {
		result = append(result, dto.LoginAgreementDocument{
			ID:        item.ID,
			Title:     item.Title,
			ContentMD: item.ContentMD,
		})
	}
	return result
}

func loginAgreementDocumentsToService(items []dto.LoginAgreementDocument) []service.LoginAgreementDocument {
	result := make([]service.LoginAgreementDocument, 0, len(items))
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		content := strings.TrimSpace(item.ContentMD)
		if title == "" && content == "" {
			continue
		}
		result = append(result, service.LoginAgreementDocument{
			ID:        strings.TrimSpace(item.ID),
			Title:     title,
			ContentMD: content,
		})
	}
	return result
}

func systemSettingsResponseData(settings dto.SystemSettings, authSourceDefaults *service.AuthSourceDefaultSettings) map[string]any {
	data := make(map[string]any)
	raw, err := json.Marshal(settings)
	if err == nil {
		_ = json.Unmarshal(raw, &data)
	}
	if authSourceDefaults == nil {
		authSourceDefaults = &service.AuthSourceDefaultSettings{}
	}

	data["auth_source_default_email_balance"] = authSourceDefaults.Email.Balance
	data["auth_source_default_email_concurrency"] = authSourceDefaults.Email.Concurrency
	data["auth_source_default_email_subscriptions"] = authSourceDefaults.Email.Subscriptions
	data["auth_source_default_email_grant_on_signup"] = authSourceDefaults.Email.GrantOnSignup
	data["auth_source_default_email_grant_on_first_bind"] = authSourceDefaults.Email.GrantOnFirstBind
	data["auth_source_default_email_platform_quotas"] = authSourceDefaults.Email.PlatformQuotas

	return data
}
