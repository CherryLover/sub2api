package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	TotpEnabled           bool  `json:"totp_enabled"`             // TOTP 双因素认证
	PasskeyEnabled        *bool `json:"passkey_enabled"`          // Passkey 登录（省略=保持现值）
	SessionBindingEnabled *bool `json:"session_binding_enabled"`  // 会话 IP/UA 绑定（省略=保持现值）
	StepUpEnabled         *bool `json:"step_up_enabled"`          // 敏感操作 step-up 2FA（省略=保持现值）
	AuditLogRetentionDays int   `json:"audit_log_retention_days"` // 审计日志保留天数

	// 登录入口 / 默认首页（省略=保持现值）。
	// 指针类型是刻意的：不带这几个字段的旧客户端/脚本做一次全量保存时，绝不能把
	// 登录入口静默重置成公开——那等于替站长撤掉了他刚藏起来的入口。
	LoginEntryPublic *bool   `json:"login_entry_public"`
	LoginEntryPath   *string `json:"login_entry_path"`
	DefaultHomePath  *string `json:"default_home_path"`

	// API Key IP 访问控制设置
	APIKeyACLTrustForwardedIP *bool     `json:"api_key_acl_trust_forwarded_ip"`
	ForwardedClientIPHeaders  *[]string `json:"forwarded_client_ip_headers"`

	// OEM设置
	DocURL          string                `json:"doc_url"`
	CustomEndpoints *[]dto.CustomEndpoint `json:"custom_endpoints"`

	// 默认配置
	DefaultConcurrency  int `json:"default_concurrency"`
	DefaultUserRPMLimit int `json:"default_user_rpm_limit"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         *bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled *bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          *string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    *int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`
	MaxClaudeCodeVersion string `json:"max_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling bool `json:"allow_ungrouped_key_scheduling"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`

	// Gateway forwarding behavior
	EnableFingerprintUnification           *bool   `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough              *bool   `json:"enable_metadata_passthrough"`
	EnableCCHSigning                       *bool   `json:"enable_cch_signing"`
	EnableClaudeOAuthSystemPromptInjection *bool   `json:"enable_claude_oauth_system_prompt_injection"`
	ClaudeOAuthSystemPrompt                *string `json:"claude_oauth_system_prompt"`
	ClaudeOAuthSystemPromptBlocks          *string `json:"claude_oauth_system_prompt_blocks"`
	EnableAnthropicCacheTTL1hInjection     *bool   `json:"enable_anthropic_cache_ttl_1h_injection"`
	RewriteMessageCacheControl             *bool   `json:"rewrite_message_cache_control"`
	EnableClientDatelineNormalization      *bool   `json:"enable_client_dateline_normalization"`
	AntigravityUserAgentVersion            *string `json:"antigravity_user_agent_version"`
	OpenAICodexUserAgent                   *string `json:"openai_codex_user_agent"`
	OpenAICodexClientVersion               *string `json:"openai_codex_client_version"`
	OpenAICodexVersionAutoSyncEnabled      *bool   `json:"openai_codex_version_auto_sync_enabled"`

	// codex_cli_only 加固（global-only）
	MinCodexVersion                      string `json:"min_codex_version"`
	MaxCodexVersion                      string `json:"max_codex_version"`
	CodexCLIOnlyBlacklist                string `json:"codex_cli_only_blacklist"`
	CodexCLIOnlyWhitelist                string `json:"codex_cli_only_whitelist"`
	CodexCLIOnlyAllowAppServerClients    *bool  `json:"codex_cli_only_allow_app_server_clients"`
	CodexCLIOnlyEngineFingerprintSignals string `json:"codex_cli_only_engine_fingerprint_signals"`

	// OpenAI account scheduling
	OpenAILowUpstreamRatePriorityEnabled               *bool    `json:"openai_low_upstream_rate_priority_enabled"`
	OpenAIOAuthSchedulingRateMultiplier                *float64 `json:"openai_oauth_scheduling_rate_multiplier"`
	OpenAIAdvancedSchedulerEnabled                     *bool    `json:"openai_advanced_scheduler_enabled"`
	OpenAIAdvancedSchedulerStickyWeightedEnabled       *bool    `json:"openai_advanced_scheduler_sticky_weighted_enabled"`
	OpenAIAdvancedSchedulerSubscriptionPriorityEnabled *bool    `json:"openai_advanced_scheduler_subscription_priority_enabled"`
	OpenAIAdvancedSchedulerLBTopK                      *string  `json:"openai_advanced_scheduler_lb_top_k"`
	OpenAIAdvancedSchedulerWeightPriority              *string  `json:"openai_advanced_scheduler_weight_priority"`
	OpenAIAdvancedSchedulerWeightLoad                  *string  `json:"openai_advanced_scheduler_weight_load"`
	OpenAIAdvancedSchedulerWeightQueue                 *string  `json:"openai_advanced_scheduler_weight_queue"`
	OpenAIAdvancedSchedulerWeightErrorRate             *string  `json:"openai_advanced_scheduler_weight_error_rate"`
	OpenAIAdvancedSchedulerWeightTTFT                  *string  `json:"openai_advanced_scheduler_weight_ttft"`
	OpenAIAdvancedSchedulerWeightReset                 *string  `json:"openai_advanced_scheduler_weight_reset"`
	OpenAIAdvancedSchedulerWeightQuotaHeadroom         *string  `json:"openai_advanced_scheduler_weight_quota_headroom"`
	OpenAIAdvancedSchedulerWeightUpstreamCost          *string  `json:"openai_advanced_scheduler_weight_upstream_cost"`
	OpenAIAdvancedSchedulerWeightPreviousResponse      *string  `json:"openai_advanced_scheduler_weight_previous_response"`
	OpenAIAdvancedSchedulerWeightSessionSticky         *string  `json:"openai_advanced_scheduler_weight_session_sticky"`

	// Channel Monitor feature switch
	ChannelMonitorEnabled        *bool `json:"channel_monitor_enabled"`
	ChannelMonitorHideThroughput *bool `json:"channel_monitor_hide_throughput"`

	// Grok model mapping policy
	GrokDefaultTextModel           *string `json:"grok_default_text_model"`
	GrokCrossClientModelMapEnabled *bool   `json:"grok_cross_client_model_map_enabled"`
	GrokDefaultBaseURLMode         *string `json:"grok_default_base_url_mode"`

	// Available Channels feature switch (user-facing)
	AvailableChannelsEnabled *bool `json:"available_channels_enabled"`

	// 风控中心功能开关
	RiskControlEnabled *bool `json:"risk_control_enabled"`

	// cyber 会话屏蔽开关 + TTL
	CyberSessionBlockEnabled    *bool `json:"cyber_session_block_enabled"`
	CyberSessionBlockTTLSeconds *int  `json:"cyber_session_block_ttl_seconds"`

	// OpenAI fast/flex policy (optional, only updated when provided)
	OpenAIFastPolicySettings *dto.OpenAIFastPolicySettings `json:"openai_fast_policy_settings,omitempty"`

	// 系统全局 platform quota 默认值（整体替换语义：nil = 不修改，non-nil = 整体覆盖）。
	DefaultPlatformQuotas map[string]*service.DefaultPlatformQuotaSetting `json:"default_platform_quotas"`

	// 各平台账号自动停调阈值（整体替换语义：nil = 不修改，non-nil = 整体覆盖）。
	AccountSchedulingThresholds map[string]int `json:"account_scheduling_thresholds"`

	// auth-source 层 platform quota 覆盖（override 语义：nil = 不修改，non-nil = 整体覆盖该 source 的 quota 配置）。

	AllowUserViewErrorRequests *bool `json:"allow_user_view_error_requests"`
}

// UpdateSettings 更新系统设置
// PUT /api/v1/admin/settings
// ensureActorTotpForStepUp 校验当前操作者具备开启 step-up 门控的条件：
// 必须是真人管理员会话（admin API key 无法完成 TOTP step-up，拒绝）且本人已启用 TOTP。
// 校验失败时写入错误响应并返回 false。
func (h *SettingHandler) ensureActorTotpForStepUp(c *gin.Context) bool {
	if c.GetString("auth_method") == service.AuditAuthMethodAdminAPIKey {
		response.ErrorWithDetails(c, http.StatusForbidden,
			"Admin API key cannot enable step-up verification; use an admin session with TOTP enabled",
			"STEP_UP_ADMIN_API_KEY_FORBIDDEN", nil)
		return false
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.ErrorWithDetails(c, http.StatusForbidden,
			"Enabling step-up verification requires an authenticated admin session",
			"STEP_UP_ENABLE_REQUIRES_TOTP", nil)
		return false
	}
	if h.userService == nil {
		response.InternalError(c, "Step-up precondition check unavailable")
		return false
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	if !user.TotpEnabled {
		response.ErrorWithDetails(c, http.StatusBadRequest,
			"Enable two-factor authentication (TOTP) for your account before turning on step-up verification",
			"STEP_UP_ENABLE_REQUIRES_TOTP", nil)
		return false
	}
	return true
}

// settingKeyJSONAliases covers the request fields whose JSON name differs from
// the setting key they persist to. Every other field of UpdateSettingsRequest
// is named after its setting key.
var settingKeyJSONAliases = map[string]string{}

// settingKeyByJSONName maps the value-typed top-level JSON fields of
// UpdateSettingsRequest to the setting key each one writes. Resolved once from
// the struct tags so new fields are covered without touching this file.
//
// Pointer-typed fields are deliberately excluded: they already carry their own
// "omitted = keep the stored value" merge in UpdateSettings, and some of them
// rely on being rewritten on every save to re-normalize fail-closed security
// state (see TestUpdateSettingsMalformedForwardedClientIPHeadersRemainFailClosedWhenOmitted).
// Only the value-typed fields are indistinguishable from a deliberate clear.
var settingKeyByJSONName = buildSettingKeyByJSONName()

func buildSettingKeyByJSONName() map[string]string {
	t := reflect.TypeOf(UpdateSettingsRequest{})
	out := make(map[string]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() == reflect.Ptr {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		if alias, ok := settingKeyJSONAliases[name]; ok {
			out[name] = alias
			continue
		}
		out[name] = name
	}
	return out
}

// omittedSettingKeys reports the setting keys this payload never mentioned.
// Saving settings is a whole-document PUT, so without this a client that sends
// only the one field it cares about resets every other field to a zero value.
func omittedSettingKeys(sentFields map[string]json.RawMessage) service.OmittedSettingKeys {
	omitted := make(service.OmittedSettingKeys, len(settingKeyByJSONName))
	for jsonName, settingKey := range settingKeyByJSONName {
		if _, sent := sentFields[jsonName]; !sent {
			omitted[settingKey] = struct{}{}
		}
	}
	return omitted
}

func settingsAuditRequest(req UpdateSettingsRequest) UpdateSettingsRequest {
	return req
}

func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var sentFields map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&sentFields, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	var req UpdateSettingsRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	auditReq := settingsAuditRequest(req)
	omitted := omittedSettingKeys(sentFields)

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 登录入口 / 默认首页：先合并 + 校验，非法值（含"隐藏但路径为空"）在这里就被拒掉，
	// 绝不允许落库——落库了就是登录页再也打不开、服务却照常运行。
	webEntryPlan, ok := h.planWebEntryUpdate(c, req, previousSettings, omitted)
	if !ok {
		return
	}

	// 两个安全开关的请求字段为指针：省略字段=保持现值，避免旧客户端/脚本
	// 用不含新字段的全量 payload 保存设置时把安全开关静默重置。
	sessionBindingEnabled := previousSettings.SessionBindingEnabled
	if req.SessionBindingEnabled != nil {
		sessionBindingEnabled = *req.SessionBindingEnabled
	}
	stepUpEnabled := previousSettings.StepUpEnabled
	if req.StepUpEnabled != nil {
		stepUpEnabled = *req.StepUpEnabled
	}
	passkeyEnabled := previousSettings.PasskeyEnabled
	if req.PasskeyEnabled != nil {
		passkeyEnabled = *req.PasskeyEnabled
	}
	if passkeyEnabled {
		configured, _, _ := h.settingService.PasskeyConfiguration()
		if !configured {
			response.BadRequest(c, "Passkey sign-in requires a valid WebAuthn RP ID and allowed HTTPS origins in the deployment configuration")
			return
		}
	}
	forwardedClientIPHeaders := append([]string(nil), previousSettings.ForwardedClientIPHeaders...)
	if req.ForwardedClientIPHeaders != nil {
		forwardedClientIPHeaders = append([]string(nil), (*req.ForwardedClientIPHeaders)...)
	}

	// 开启敏感操作 step-up 门控属自锁风险操作：仅允许本人已启用 TOTP 的管理员会话开启，
	// 否则开启后操作者立即被挡在所有敏感操作之外。仅在 false→true 的开启瞬间校验，
	// 保持开启状态的常规设置保存不受影响。
	if stepUpEnabled && !previousSettings.StepUpEnabled {
		if !h.ensureActorTotpForStepUp(c) {
			return
		}
	}
	// 关闭 step-up 门控本身就是敏感操作：防止拿到管理员会话的攻击者先关闸再执行导出/备份。
	// previousSettings 已证实开关处于开启状态，使用无条件门控变体，
	// 避免门控内部二次读取开关时因存储故障 fail-open（前端捕获 STEP_UP_REQUIRED 弹码重试）。
	if !stepUpEnabled && previousSettings.StepUpEnabled {
		if !middleware.EnforceStepUpAlways(c, h.totpService, h.userService) {
			return
		}
	}

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}

	// TOTP 双因素认证参数验证
	// 只有手动配置了加密密钥才允许启用 TOTP 功能
	if req.TotpEnabled && !previousSettings.TotpEnabled {
		// 尝试启用 TOTP，检查加密密钥是否已手动配置
		if !h.settingService.IsTotpEncryptionKeyConfigured() {
			response.BadRequest(c, "Cannot enable TOTP: TOTP_ENCRYPTION_KEY environment variable must be configured first. Generate a key with 'openssl rand -hex 32' and set it in your environment.")
			return
		}
	}
	// 自定义端点验证
	const (
		maxCustomEndpoints        = 10
		maxEndpointNameLen        = 50
		maxEndpointURLLen         = 2048
		maxEndpointDescriptionLen = 200
	)

	customEndpointsJSON := previousSettings.CustomEndpoints
	if req.CustomEndpoints != nil {
		endpoints := *req.CustomEndpoints
		if len(endpoints) > maxCustomEndpoints {
			response.BadRequest(c, "Too many custom endpoints (max 10)")
			return
		}
		for _, ep := range endpoints {
			if strings.TrimSpace(ep.Name) == "" {
				response.BadRequest(c, "Custom endpoint name is required")
				return
			}
			if len(ep.Name) > maxEndpointNameLen {
				response.BadRequest(c, "Custom endpoint name is too long (max 50 characters)")
				return
			}
			if strings.TrimSpace(ep.Endpoint) == "" {
				response.BadRequest(c, "Custom endpoint URL is required")
				return
			}
			if len(ep.Endpoint) > maxEndpointURLLen {
				response.BadRequest(c, "Custom endpoint URL is too long (max 2048 characters)")
				return
			}
			if err := config.ValidateAbsoluteHTTPURL(strings.TrimSpace(ep.Endpoint)); err != nil {
				response.BadRequest(c, "Custom endpoint URL must be an absolute http(s) URL")
				return
			}
			if len(ep.Description) > maxEndpointDescriptionLen {
				response.BadRequest(c, "Custom endpoint description is too long (max 200 characters)")
				return
			}
		}
		endpointBytes, err := json.Marshal(endpoints)
		if err != nil {
			response.BadRequest(c, "Failed to serialize custom endpoints")
			return
		}
		customEndpointsJSON = string(endpointBytes)
	}

	// Ops metrics collector interval validation (seconds).
	if req.OpsMetricsIntervalSeconds != nil {
		v := *req.OpsMetricsIntervalSeconds
		if v < 60 {
			v = 60
		}
		if v > 3600 {
			v = 3600
		}
		req.OpsMetricsIntervalSeconds = &v
	}
	// 验证最低版本号格式（空字符串=禁用，或合法 semver）
	if req.MinClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MinClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "min_claude_code_version must be empty or a valid semver (e.g. 2.1.63)")
			return
		}
	}

	// 验证最高版本号格式（空字符串=禁用，或合法 semver）
	if req.MaxClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MaxClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be empty or a valid semver (e.g. 3.0.0)")
			return
		}
	}
	if req.AntigravityUserAgentVersion != nil {
		normalized := strings.TrimSpace(*req.AntigravityUserAgentVersion)
		req.AntigravityUserAgentVersion = &normalized
		if normalized != "" && !semverPattern.MatchString(normalized) {
			response.Error(c, http.StatusBadRequest, "antigravity_user_agent_version must be empty or a valid semver (e.g. 1.23.2)")
			return
		}
	}
	if req.OpenAICodexUserAgent != nil {
		normalized := strings.TrimSpace(*req.OpenAICodexUserAgent)
		req.OpenAICodexUserAgent = &normalized
		// 仅做长度上限保护，不限制具体格式（运维需要可自由调整 codex 版本号）
		if len(normalized) > 512 {
			response.Error(c, http.StatusBadRequest, "openai_codex_user_agent must be at most 512 characters")
			return
		}
	}
	if req.OpenAICodexClientVersion != nil {
		// 该值会被拼进出站 User-Agent 与 version 头，必须是合法版本号；空串表示跟随自动同步。
		normalized := strings.TrimSpace(*req.OpenAICodexClientVersion)
		if normalized != "" && service.NormalizeCodexClientVersion(normalized) == "" {
			response.Error(c, http.StatusBadRequest, "openai_codex_client_version must be empty or a valid version (e.g. 0.146.0)")
			return
		}
		req.OpenAICodexClientVersion = &normalized
	}

	// codex_cli_only 加固：最低/最高 Codex 版本（空=禁用，或合法 semver；max>=min）
	if req.MinCodexVersion != "" && !semverPattern.MatchString(req.MinCodexVersion) {
		response.Error(c, http.StatusBadRequest, "min_codex_version must be empty or a valid semver (e.g. 0.141.0)")
		return
	}
	if req.MaxCodexVersion != "" && !semverPattern.MatchString(req.MaxCodexVersion) {
		response.Error(c, http.StatusBadRequest, "max_codex_version must be empty or a valid semver (e.g. 0.200.0)")
		return
	}
	if req.MinCodexVersion != "" && req.MaxCodexVersion != "" && service.CompareVersions(req.MaxCodexVersion, req.MinCodexVersion) < 0 {
		response.Error(c, http.StatusBadRequest, "max_codex_version must be greater than or equal to min_codex_version")
		return
	}
	// codex_cli_only 黑/白名单：非空须为合法 []AllowedClientEntry JSON。
	// 黑名单 OR 宽 deny（允许 originator-only）；白名单双因子 AND，额外要求每条可命中（非空 originator + ua_contains）。
	if err := service.ValidateCodexClientEntriesJSON(req.CodexCLIOnlyBlacklist); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_blacklist "+err.Error())
		return
	}
	if err := service.ValidateCodexWhitelistEntriesJSON(req.CodexCLIOnlyWhitelist); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_whitelist "+err.Error())
		return
	}
	if err := service.ValidateEngineFingerprintSignalsJSON(req.CodexCLIOnlyEngineFingerprintSignals); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_engine_fingerprint_signals "+err.Error())
		return
	}

	// 交叉验证：如果同时设置了最低和最高版本号，最高版本号必须 >= 最低版本号
	if req.MinClaudeCodeVersion != "" && req.MaxClaudeCodeVersion != "" {
		if service.CompareVersions(req.MaxClaudeCodeVersion, req.MinClaudeCodeVersion) < 0 {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be greater than or equal to min_claude_code_version")
			return
		}
	}

	// cyber 会话屏蔽 TTL 校验：提供时必须 > 0
	if req.CyberSessionBlockTTLSeconds != nil && *req.CyberSessionBlockTTLSeconds <= 0 {
		response.BadRequest(c, "cyber_session_block_ttl_seconds must be > 0")
		return
	}

	settings := &service.SystemSettings{
		// 系统全局 platform quota 默认值（整体替换语义）
		DefaultPlatformQuotas:       req.DefaultPlatformQuotas,
		AccountSchedulingThresholds: req.AccountSchedulingThresholds,

		TotpEnabled:           req.TotpEnabled,
		PasskeyEnabled:        passkeyEnabled,
		SessionBindingEnabled: sessionBindingEnabled,
		StepUpEnabled:         stepUpEnabled,
		LoginEntryPublic:      webEntryPlan.LoginEntryPublic,
		LoginEntryPath:        webEntryPlan.LoginEntryPath,
		DefaultHomePath:       webEntryPlan.DefaultHomePath,
		AuditLogRetentionDays: req.AuditLogRetentionDays,
		APIKeyACLTrustForwardedIP: func() bool {
			if req.APIKeyACLTrustForwardedIP != nil {
				return *req.APIKeyACLTrustForwardedIP
			}
			return previousSettings.APIKeyACLTrustForwardedIP
		}(),
		ForwardedClientIPHeaders:    forwardedClientIPHeaders,
		DocURL:                      req.DocURL,
		CustomEndpoints:             customEndpointsJSON,
		DefaultConcurrency:          req.DefaultConcurrency,
		DefaultUserRPMLimit:         req.DefaultUserRPMLimit,
		EnableModelFallback:         req.EnableModelFallback,
		FallbackModelAnthropic:      req.FallbackModelAnthropic,
		FallbackModelOpenAI:         req.FallbackModelOpenAI,
		FallbackModelGemini:         req.FallbackModelGemini,
		FallbackModelAntigravity:    req.FallbackModelAntigravity,
		EnableIdentityPatch:         req.EnableIdentityPatch,
		IdentityPatchPrompt:         req.IdentityPatchPrompt,
		MinClaudeCodeVersion:        req.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:        req.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling: req.AllowUngroupedKeyScheduling,
		BackendModeEnabled:          req.BackendModeEnabled,
		AllowUserViewErrorRequests: func() bool {
			if req.AllowUserViewErrorRequests != nil {
				return *req.AllowUserViewErrorRequests
			}
			return previousSettings.AllowUserViewErrorRequests
		}(),
		OpsMonitoringEnabled: func() bool {
			if req.OpsMonitoringEnabled != nil {
				return *req.OpsMonitoringEnabled
			}
			return previousSettings.OpsMonitoringEnabled
		}(),
		OpsRealtimeMonitoringEnabled: func() bool {
			if req.OpsRealtimeMonitoringEnabled != nil {
				return *req.OpsRealtimeMonitoringEnabled
			}
			return previousSettings.OpsRealtimeMonitoringEnabled
		}(),
		OpsQueryModeDefault: func() string {
			if req.OpsQueryModeDefault != nil {
				return *req.OpsQueryModeDefault
			}
			return previousSettings.OpsQueryModeDefault
		}(),
		OpsMetricsIntervalSeconds: func() int {
			if req.OpsMetricsIntervalSeconds != nil {
				return *req.OpsMetricsIntervalSeconds
			}
			return previousSettings.OpsMetricsIntervalSeconds
		}(),
		EnableFingerprintUnification: func() bool {
			if req.EnableFingerprintUnification != nil {
				return *req.EnableFingerprintUnification
			}
			return previousSettings.EnableFingerprintUnification
		}(),
		EnableMetadataPassthrough: func() bool {
			if req.EnableMetadataPassthrough != nil {
				return *req.EnableMetadataPassthrough
			}
			return previousSettings.EnableMetadataPassthrough
		}(),
		EnableCCHSigning: func() bool {
			if req.EnableCCHSigning != nil {
				return *req.EnableCCHSigning
			}
			return previousSettings.EnableCCHSigning
		}(),
		EnableClaudeOAuthSystemPromptInjection: func() bool {
			if req.EnableClaudeOAuthSystemPromptInjection != nil {
				return *req.EnableClaudeOAuthSystemPromptInjection
			}
			return previousSettings.EnableClaudeOAuthSystemPromptInjection
		}(),
		ClaudeOAuthSystemPrompt: func() string {
			if req.ClaudeOAuthSystemPrompt != nil {
				return *req.ClaudeOAuthSystemPrompt
			}
			return previousSettings.ClaudeOAuthSystemPrompt
		}(),
		ClaudeOAuthSystemPromptBlocks: func() string {
			if req.ClaudeOAuthSystemPromptBlocks != nil {
				return *req.ClaudeOAuthSystemPromptBlocks
			}
			return previousSettings.ClaudeOAuthSystemPromptBlocks
		}(),
		EnableAnthropicCacheTTL1hInjection: func() bool {
			if req.EnableAnthropicCacheTTL1hInjection != nil {
				return *req.EnableAnthropicCacheTTL1hInjection
			}
			return previousSettings.EnableAnthropicCacheTTL1hInjection
		}(),
		RewriteMessageCacheControl: func() bool {
			if req.RewriteMessageCacheControl != nil {
				return *req.RewriteMessageCacheControl
			}
			return previousSettings.RewriteMessageCacheControl
		}(),
		EnableClientDatelineNormalization: func() bool {
			if req.EnableClientDatelineNormalization != nil {
				return *req.EnableClientDatelineNormalization
			}
			return previousSettings.EnableClientDatelineNormalization
		}(),
		AntigravityUserAgentVersion: func() string {
			if req.AntigravityUserAgentVersion != nil {
				return *req.AntigravityUserAgentVersion
			}
			return previousSettings.AntigravityUserAgentVersion
		}(),
		OpenAICodexUserAgent: func() string {
			if req.OpenAICodexUserAgent != nil {
				return *req.OpenAICodexUserAgent
			}
			return previousSettings.OpenAICodexUserAgent
		}(),
		OpenAICodexClientVersion: func() string {
			if req.OpenAICodexClientVersion != nil {
				return *req.OpenAICodexClientVersion
			}
			return previousSettings.OpenAICodexClientVersion
		}(),
		// 同步值由自动同步任务独占写入，面板保存时原样带回，避免被清空。
		OpenAICodexClientVersionSynced: previousSettings.OpenAICodexClientVersionSynced,
		OpenAICodexVersionAutoSyncEnabled: func() bool {
			if req.OpenAICodexVersionAutoSyncEnabled != nil {
				return *req.OpenAICodexVersionAutoSyncEnabled
			}
			return previousSettings.OpenAICodexVersionAutoSyncEnabled
		}(),
		MinCodexVersion:       strings.TrimSpace(req.MinCodexVersion),
		MaxCodexVersion:       strings.TrimSpace(req.MaxCodexVersion),
		CodexCLIOnlyBlacklist: strings.TrimSpace(req.CodexCLIOnlyBlacklist),
		CodexCLIOnlyWhitelist: strings.TrimSpace(req.CodexCLIOnlyWhitelist),
		CodexCLIOnlyAllowAppServerClients: func() bool {
			if req.CodexCLIOnlyAllowAppServerClients != nil {
				return *req.CodexCLIOnlyAllowAppServerClients
			}
			return previousSettings.CodexCLIOnlyAllowAppServerClients
		}(),
		CodexCLIOnlyEngineFingerprintSignals: strings.TrimSpace(req.CodexCLIOnlyEngineFingerprintSignals),
		OpenAILowUpstreamRatePriorityEnabled: func() bool {
			if req.OpenAILowUpstreamRatePriorityEnabled != nil {
				return *req.OpenAILowUpstreamRatePriorityEnabled
			}
			return previousSettings.OpenAILowUpstreamRatePriorityEnabled
		}(),
		OpenAIOAuthSchedulingRateMultiplier: func() float64 {
			if req.OpenAIOAuthSchedulingRateMultiplier != nil {
				return *req.OpenAIOAuthSchedulingRateMultiplier
			}
			return previousSettings.OpenAIOAuthSchedulingRateMultiplier
		}(),
		OpenAIAdvancedSchedulerEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerEnabled != nil {
				return *req.OpenAIAdvancedSchedulerEnabled
			}
			return previousSettings.OpenAIAdvancedSchedulerEnabled
		}(),
		OpenAIAdvancedSchedulerStickyWeightedEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerStickyWeightedEnabled != nil {
				return *req.OpenAIAdvancedSchedulerStickyWeightedEnabled
			}
			return previousSettings.OpenAIAdvancedSchedulerStickyWeightedEnabled
		}(),
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled: func() bool {
			if req.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled != nil {
				return *req.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled
			}
			return previousSettings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled
		}(),
		OpenAIAdvancedSchedulerLBTopK:                 stringSetting(req.OpenAIAdvancedSchedulerLBTopK, previousSettings.OpenAIAdvancedSchedulerLBTopK),
		OpenAIAdvancedSchedulerWeightPriority:         stringSetting(req.OpenAIAdvancedSchedulerWeightPriority, previousSettings.OpenAIAdvancedSchedulerWeightPriority),
		OpenAIAdvancedSchedulerWeightLoad:             stringSetting(req.OpenAIAdvancedSchedulerWeightLoad, previousSettings.OpenAIAdvancedSchedulerWeightLoad),
		OpenAIAdvancedSchedulerWeightQueue:            stringSetting(req.OpenAIAdvancedSchedulerWeightQueue, previousSettings.OpenAIAdvancedSchedulerWeightQueue),
		OpenAIAdvancedSchedulerWeightErrorRate:        stringSetting(req.OpenAIAdvancedSchedulerWeightErrorRate, previousSettings.OpenAIAdvancedSchedulerWeightErrorRate),
		OpenAIAdvancedSchedulerWeightTTFT:             stringSetting(req.OpenAIAdvancedSchedulerWeightTTFT, previousSettings.OpenAIAdvancedSchedulerWeightTTFT),
		OpenAIAdvancedSchedulerWeightReset:            stringSetting(req.OpenAIAdvancedSchedulerWeightReset, previousSettings.OpenAIAdvancedSchedulerWeightReset),
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:    stringSetting(req.OpenAIAdvancedSchedulerWeightQuotaHeadroom, previousSettings.OpenAIAdvancedSchedulerWeightQuotaHeadroom),
		OpenAIAdvancedSchedulerWeightUpstreamCost:     stringSetting(req.OpenAIAdvancedSchedulerWeightUpstreamCost, previousSettings.OpenAIAdvancedSchedulerWeightUpstreamCost),
		OpenAIAdvancedSchedulerWeightPreviousResponse: stringSetting(req.OpenAIAdvancedSchedulerWeightPreviousResponse, previousSettings.OpenAIAdvancedSchedulerWeightPreviousResponse),
		OpenAIAdvancedSchedulerWeightSessionSticky:    stringSetting(req.OpenAIAdvancedSchedulerWeightSessionSticky, previousSettings.OpenAIAdvancedSchedulerWeightSessionSticky),
		ChannelMonitorEnabled: func() bool {
			if req.ChannelMonitorEnabled != nil {
				return *req.ChannelMonitorEnabled
			}
			return previousSettings.ChannelMonitorEnabled
		}(),
		ChannelMonitorHideThroughput: func() bool {
			if req.ChannelMonitorHideThroughput != nil {
				return *req.ChannelMonitorHideThroughput
			}
			return previousSettings.ChannelMonitorHideThroughput
		}(),
		GrokDefaultTextModel: func() string {
			if req.GrokDefaultTextModel != nil {
				return *req.GrokDefaultTextModel
			}
			return previousSettings.GrokDefaultTextModel
		}(),
		GrokCrossClientModelMapEnabled: func() bool {
			if req.GrokCrossClientModelMapEnabled != nil {
				return *req.GrokCrossClientModelMapEnabled
			}
			return previousSettings.GrokCrossClientModelMapEnabled
		}(),
		GrokDefaultBaseURLMode: func() string {
			if req.GrokDefaultBaseURLMode != nil {
				return strings.TrimSpace(*req.GrokDefaultBaseURLMode)
			}
			return previousSettings.GrokDefaultBaseURLMode
		}(),
		AvailableChannelsEnabled: func() bool {
			if req.AvailableChannelsEnabled != nil {
				return *req.AvailableChannelsEnabled
			}
			return previousSettings.AvailableChannelsEnabled
		}(),
		RiskControlEnabled: func() bool {
			if req.RiskControlEnabled != nil {
				return *req.RiskControlEnabled
			}
			return previousSettings.RiskControlEnabled
		}(),
		CyberSessionBlockEnabled: func() bool {
			if req.CyberSessionBlockEnabled != nil {
				return *req.CyberSessionBlockEnabled
			}
			return previousSettings.CyberSessionBlockEnabled
		}(),
		CyberSessionBlockTTLSeconds: func() int {
			if req.CyberSessionBlockTTLSeconds != nil {
				return *req.CyberSessionBlockTTLSeconds
			}
			return previousSettings.CyberSessionBlockTTLSeconds
		}(),
	}

	if err := h.settingService.UpdateSettingsOmitting(c.Request.Context(), settings, omitted); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.opsService != nil {
		h.opsService.SetMonitoringEnabled(settings.OpsMonitoringEnabled)
	}

	// Update OpenAI fast policy (stored under dedicated key, only when provided).
	if req.OpenAIFastPolicySettings != nil {
		if err := h.settingService.SetOpenAIFastPolicySettings(c.Request.Context(), openaiFastPolicySettingsFromDTO(req.OpenAIFastPolicySettings)); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}

	h.auditSettingsUpdate(c, previousSettings, settings, auditReq)

	// 重新获取设置返回
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	passkeyConfigured, passkeyRPID, passkeyRPOrigins := h.settingService.PasskeyConfiguration()

	payload := dto.SystemSettings{
		TotpEnabled:                                            updatedSettings.TotpEnabled,
		TotpEncryptionKeyConfigured:                            h.settingService.IsTotpEncryptionKeyConfigured(),
		PasskeyEnabled:                                         updatedSettings.PasskeyEnabled,
		PasskeyConfigured:                                      passkeyConfigured,
		PasskeyRPID:                                            passkeyRPID,
		PasskeyRPOrigins:                                       passkeyRPOrigins,
		SessionBindingEnabled:                                  updatedSettings.SessionBindingEnabled,
		StepUpEnabled:                                          updatedSettings.StepUpEnabled,
		AuditLogRetentionDays:                                  updatedSettings.AuditLogRetentionDays,
		APIKeyACLTrustForwardedIP:                              updatedSettings.APIKeyACLTrustForwardedIP,
		ForwardedClientIPHeaders:                               updatedSettings.ForwardedClientIPHeaders,
		DocURL:                                                 updatedSettings.DocURL,
		CustomEndpoints:                                        dto.ParseCustomEndpoints(updatedSettings.CustomEndpoints),
		DefaultConcurrency:                                     updatedSettings.DefaultConcurrency,
		DefaultUserRPMLimit:                                    updatedSettings.DefaultUserRPMLimit,
		EnableModelFallback:                                    updatedSettings.EnableModelFallback,
		FallbackModelAnthropic:                                 updatedSettings.FallbackModelAnthropic,
		FallbackModelOpenAI:                                    updatedSettings.FallbackModelOpenAI,
		FallbackModelGemini:                                    updatedSettings.FallbackModelGemini,
		FallbackModelAntigravity:                               updatedSettings.FallbackModelAntigravity,
		EnableIdentityPatch:                                    updatedSettings.EnableIdentityPatch,
		IdentityPatchPrompt:                                    updatedSettings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                                   updatedSettings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:                           updatedSettings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                                    updatedSettings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:                              updatedSettings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                                   updatedSettings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                                   updatedSettings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:                            updatedSettings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                                     updatedSettings.BackendModeEnabled,
		EnableFingerprintUnification:                           updatedSettings.EnableFingerprintUnification,
		EnableMetadataPassthrough:                              updatedSettings.EnableMetadataPassthrough,
		EnableCCHSigning:                                       updatedSettings.EnableCCHSigning,
		EnableClaudeOAuthSystemPromptInjection:                 updatedSettings.EnableClaudeOAuthSystemPromptInjection,
		ClaudeOAuthSystemPrompt:                                updatedSettings.ClaudeOAuthSystemPrompt,
		ClaudeOAuthSystemPromptBlocks:                          updatedSettings.ClaudeOAuthSystemPromptBlocks,
		EnableAnthropicCacheTTL1hInjection:                     updatedSettings.EnableAnthropicCacheTTL1hInjection,
		RewriteMessageCacheControl:                             updatedSettings.RewriteMessageCacheControl,
		EnableClientDatelineNormalization:                      updatedSettings.EnableClientDatelineNormalization,
		AntigravityUserAgentVersion:                            updatedSettings.AntigravityUserAgentVersion,
		OpenAICodexUserAgent:                                   updatedSettings.OpenAICodexUserAgent,
		OpenAICodexClientVersion:                               updatedSettings.OpenAICodexClientVersion,
		OpenAICodexClientVersionSynced:                         updatedSettings.OpenAICodexClientVersionSynced,
		OpenAICodexVersionAutoSyncEnabled:                      updatedSettings.OpenAICodexVersionAutoSyncEnabled,
		MinCodexVersion:                                        updatedSettings.MinCodexVersion,
		MaxCodexVersion:                                        updatedSettings.MaxCodexVersion,
		CodexCLIOnlyBlacklist:                                  updatedSettings.CodexCLIOnlyBlacklist,
		CodexCLIOnlyWhitelist:                                  updatedSettings.CodexCLIOnlyWhitelist,
		CodexCLIOnlyAllowAppServerClients:                      updatedSettings.CodexCLIOnlyAllowAppServerClients,
		CodexCLIOnlyEngineFingerprintSignals:                   updatedSettings.CodexCLIOnlyEngineFingerprintSignals,
		OpenAILowUpstreamRatePriorityEnabled:                   updatedSettings.OpenAILowUpstreamRatePriorityEnabled,
		OpenAIOAuthSchedulingRateMultiplier:                    updatedSettings.OpenAIOAuthSchedulingRateMultiplier,
		OpenAIAdvancedSchedulerEnabled:                         updatedSettings.OpenAIAdvancedSchedulerEnabled,
		OpenAIAdvancedSchedulerStickyWeightedEnabled:           updatedSettings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled:     updatedSettings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		OpenAIAdvancedSchedulerLBTopK:                          updatedSettings.OpenAIAdvancedSchedulerLBTopK,
		OpenAIAdvancedSchedulerWeightPriority:                  updatedSettings.OpenAIAdvancedSchedulerWeightPriority,
		OpenAIAdvancedSchedulerWeightLoad:                      updatedSettings.OpenAIAdvancedSchedulerWeightLoad,
		OpenAIAdvancedSchedulerWeightQueue:                     updatedSettings.OpenAIAdvancedSchedulerWeightQueue,
		OpenAIAdvancedSchedulerWeightErrorRate:                 updatedSettings.OpenAIAdvancedSchedulerWeightErrorRate,
		OpenAIAdvancedSchedulerWeightTTFT:                      updatedSettings.OpenAIAdvancedSchedulerWeightTTFT,
		OpenAIAdvancedSchedulerWeightReset:                     updatedSettings.OpenAIAdvancedSchedulerWeightReset,
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:             updatedSettings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerWeightUpstreamCost:              updatedSettings.OpenAIAdvancedSchedulerWeightUpstreamCost,
		OpenAIAdvancedSchedulerWeightPreviousResponse:          updatedSettings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		OpenAIAdvancedSchedulerWeightSessionSticky:             updatedSettings.OpenAIAdvancedSchedulerWeightSessionSticky,
		OpenAIAdvancedSchedulerEffectiveLBTopK:                 updatedSettings.OpenAIAdvancedSchedulerEffectiveLBTopK,
		OpenAIAdvancedSchedulerEffectiveWeightPriority:         updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightPriority,
		OpenAIAdvancedSchedulerEffectiveWeightLoad:             updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightLoad,
		OpenAIAdvancedSchedulerEffectiveWeightQueue:            updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightQueue,
		OpenAIAdvancedSchedulerEffectiveWeightErrorRate:        updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightErrorRate,
		OpenAIAdvancedSchedulerEffectiveWeightTTFT:             updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightTTFT,
		OpenAIAdvancedSchedulerEffectiveWeightReset:            updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightReset,
		OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom:    updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost:     updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost,
		OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse: updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse,
		OpenAIAdvancedSchedulerEffectiveWeightSessionSticky:    updatedSettings.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky,

		ChannelMonitorEnabled:        updatedSettings.ChannelMonitorEnabled,
		ChannelMonitorHideThroughput: updatedSettings.ChannelMonitorHideThroughput,

		GrokDefaultTextModel:           updatedSettings.GrokDefaultTextModel,
		GrokCrossClientModelMapEnabled: updatedSettings.GrokCrossClientModelMapEnabled,
		GrokDefaultBaseURLMode:         updatedSettings.GrokDefaultBaseURLMode,

		AvailableChannelsEnabled: updatedSettings.AvailableChannelsEnabled,

		RiskControlEnabled:          updatedSettings.RiskControlEnabled,
		CyberSessionBlockEnabled:    updatedSettings.CyberSessionBlockEnabled,
		CyberSessionBlockTTLSeconds: updatedSettings.CyberSessionBlockTTLSeconds,
		AccountSchedulingThresholds: updatedSettings.AccountSchedulingThresholds,
		AllowUserViewErrorRequests:  updatedSettings.AllowUserViewErrorRequests,
	}
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
	} else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
	}

	// Default platform quotas（JSON map）—— 与 GetSettings 一致，避免保存后响应缺失该字段
	if platformQuotas, err := h.settingService.GetDefaultPlatformQuotas(c.Request.Context()); err != nil {
		slog.Error("default_platform_quotas_get_failed", "error", err)
	} else {
		payload.DefaultPlatformQuotas = platformQuotas
	}
	// 与 GetSettings 一致：回三层合并后的生效值，保存后界面立刻能回显真实的登录入口。
	webEntrySettingsToDTO(h.settingService.ResolveWebEntry(c.Request.Context()), &payload)

	response.Success(c, systemSettingsResponseData(payload))
}
