package dto

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CustomEndpoint represents an admin-configured API endpoint for quick copy.
type CustomEndpoint struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

// SystemSettings represents the admin settings API response payload.
type SystemSettings struct {
	RegistrationEmailSuffixWhitelist    []string `json:"registration_email_suffix_whitelist"`
	RegistrationEmailDomainQuotaEnabled bool     `json:"registration_email_domain_quota_enabled"`
	TotpEnabled                         bool     `json:"totp_enabled"`                   // TOTP 双因素认证
	TotpEncryptionKeyConfigured         bool     `json:"totp_encryption_key_configured"` // TOTP 加密密钥是否已配置
	PasskeyEnabled                      bool     `json:"passkey_enabled"`
	PasskeyConfigured                   bool     `json:"passkey_configured"`
	PasskeyRPID                         string   `json:"passkey_rp_id"`
	PasskeyRPOrigins                    []string `json:"passkey_rp_origins"`
	SessionBindingEnabled               bool     `json:"session_binding_enabled"`  // 会话 IP/UA 绑定
	StepUpEnabled                       bool     `json:"step_up_enabled"`          // 敏感操作 step-up 2FA
	AuditLogRetentionDays               int      `json:"audit_log_retention_days"` // 审计日志保留天数

	// 登录入口 / 默认首页（三层合并后的**生效值**）。
	//
	// LoginEntryPath 是这份结构里唯一"只能给管理员看"的字段：/api/v1/admin/settings
	// 走管理员鉴权，而公开设置 DTO（PublicSettings）里没有、也永远不许有这个字段。
	// LockedByConfig 为 true 表示该项被本地配置文件锁定，后台只读。
	LoginEntryPublic              bool   `json:"login_entry_public"`
	LoginEntryPath                string `json:"login_entry_path"`
	DefaultHomePath               string `json:"default_home_path"`
	LoginEntryLockedByConfig      bool   `json:"login_entry_locked_by_config"`
	DefaultHomePathLockedByConfig bool   `json:"default_home_path_locked_by_config"`

	APIKeyACLTrustForwardedIP bool     `json:"api_key_acl_trust_forwarded_ip"`
	ForwardedClientIPHeaders  []string `json:"forwarded_client_ip_headers"`

	DocURL          string           `json:"doc_url"`
	CustomEndpoints []CustomEndpoint `json:"custom_endpoints"`

	DefaultConcurrency   int                          `json:"default_concurrency"`
	DefaultBalance       float64                      `json:"default_balance"`
	DefaultUserRPMLimit  int                          `json:"default_user_rpm_limit"`
	DefaultSubscriptions []DefaultSubscriptionSetting `json:"default_subscriptions"`

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
	OpsMonitoringEnabled         bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`
	MaxClaudeCodeVersion string `json:"max_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling bool `json:"allow_ungrouped_key_scheduling"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`

	// Gateway forwarding behavior
	EnableFingerprintUnification           bool   `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough              bool   `json:"enable_metadata_passthrough"`
	EnableCCHSigning                       bool   `json:"enable_cch_signing"`
	EnableClaudeOAuthSystemPromptInjection bool   `json:"enable_claude_oauth_system_prompt_injection"`
	ClaudeOAuthSystemPrompt                string `json:"claude_oauth_system_prompt"`
	ClaudeOAuthSystemPromptBlocks          string `json:"claude_oauth_system_prompt_blocks"`
	EnableAnthropicCacheTTL1hInjection     bool   `json:"enable_anthropic_cache_ttl_1h_injection"`
	RewriteMessageCacheControl             bool   `json:"rewrite_message_cache_control"`
	EnableClientDatelineNormalization      bool   `json:"enable_client_dateline_normalization"`
	AntigravityUserAgentVersion            string `json:"antigravity_user_agent_version"`
	OpenAICodexUserAgent                   string `json:"openai_codex_user_agent"`
	OpenAICodexClientVersion               string `json:"openai_codex_client_version"`
	OpenAICodexClientVersionSynced         string `json:"openai_codex_client_version_synced"`
	OpenAICodexVersionAutoSyncEnabled      bool   `json:"openai_codex_version_auto_sync_enabled"`

	// codex_cli_only 加固
	MinCodexVersion                      string `json:"min_codex_version"`
	MaxCodexVersion                      string `json:"max_codex_version"`
	CodexCLIOnlyBlacklist                string `json:"codex_cli_only_blacklist"`
	CodexCLIOnlyWhitelist                string `json:"codex_cli_only_whitelist"`
	CodexCLIOnlyAllowAppServerClients    bool   `json:"codex_cli_only_allow_app_server_clients"`
	CodexCLIOnlyEngineFingerprintSignals string `json:"codex_cli_only_engine_fingerprint_signals"`

	// Web Search Emulation
	WebSearchEmulationEnabled bool `json:"web_search_emulation_enabled"`

	// OpenAI account scheduling
	OpenAILowUpstreamRatePriorityEnabled                   bool    `json:"openai_low_upstream_rate_priority_enabled"`
	OpenAIOAuthSchedulingRateMultiplier                    float64 `json:"openai_oauth_scheduling_rate_multiplier"`
	OpenAIAdvancedSchedulerEnabled                         bool    `json:"openai_advanced_scheduler_enabled"`
	OpenAIAdvancedSchedulerStickyWeightedEnabled           bool    `json:"openai_advanced_scheduler_sticky_weighted_enabled"`
	OpenAIAdvancedSchedulerSubscriptionPriorityEnabled     bool    `json:"openai_advanced_scheduler_subscription_priority_enabled"`
	OpenAIAdvancedSchedulerLBTopK                          string  `json:"openai_advanced_scheduler_lb_top_k"`
	OpenAIAdvancedSchedulerWeightPriority                  string  `json:"openai_advanced_scheduler_weight_priority"`
	OpenAIAdvancedSchedulerWeightLoad                      string  `json:"openai_advanced_scheduler_weight_load"`
	OpenAIAdvancedSchedulerWeightQueue                     string  `json:"openai_advanced_scheduler_weight_queue"`
	OpenAIAdvancedSchedulerWeightErrorRate                 string  `json:"openai_advanced_scheduler_weight_error_rate"`
	OpenAIAdvancedSchedulerWeightTTFT                      string  `json:"openai_advanced_scheduler_weight_ttft"`
	OpenAIAdvancedSchedulerWeightReset                     string  `json:"openai_advanced_scheduler_weight_reset"`
	OpenAIAdvancedSchedulerWeightQuotaHeadroom             string  `json:"openai_advanced_scheduler_weight_quota_headroom"`
	OpenAIAdvancedSchedulerWeightUpstreamCost              string  `json:"openai_advanced_scheduler_weight_upstream_cost"`
	OpenAIAdvancedSchedulerWeightPreviousResponse          string  `json:"openai_advanced_scheduler_weight_previous_response"`
	OpenAIAdvancedSchedulerWeightSessionSticky             string  `json:"openai_advanced_scheduler_weight_session_sticky"`
	OpenAIAdvancedSchedulerEffectiveLBTopK                 string  `json:"openai_advanced_scheduler_effective_lb_top_k"`
	OpenAIAdvancedSchedulerEffectiveWeightPriority         string  `json:"openai_advanced_scheduler_effective_weight_priority"`
	OpenAIAdvancedSchedulerEffectiveWeightLoad             string  `json:"openai_advanced_scheduler_effective_weight_load"`
	OpenAIAdvancedSchedulerEffectiveWeightQueue            string  `json:"openai_advanced_scheduler_effective_weight_queue"`
	OpenAIAdvancedSchedulerEffectiveWeightErrorRate        string  `json:"openai_advanced_scheduler_effective_weight_error_rate"`
	OpenAIAdvancedSchedulerEffectiveWeightTTFT             string  `json:"openai_advanced_scheduler_effective_weight_ttft"`
	OpenAIAdvancedSchedulerEffectiveWeightReset            string  `json:"openai_advanced_scheduler_effective_weight_reset"`
	OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom    string  `json:"openai_advanced_scheduler_effective_weight_quota_headroom"`
	OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost     string  `json:"openai_advanced_scheduler_effective_weight_upstream_cost"`
	OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse string  `json:"openai_advanced_scheduler_effective_weight_previous_response"`
	OpenAIAdvancedSchedulerEffectiveWeightSessionSticky    string  `json:"openai_advanced_scheduler_effective_weight_session_sticky"`

	// Channel Monitor feature switch
	ChannelMonitorEnabled                bool   `json:"channel_monitor_enabled"`
	ChannelMonitorMode                   string `json:"channel_monitor_mode"`
	ChannelMonitorDefaultIntervalSeconds int    `json:"channel_monitor_default_interval_seconds"`
	ChannelMonitorHideThroughput         bool   `json:"channel_monitor_hide_throughput"`
	ChannelMonitorShowQuota              bool   `json:"channel_monitor_show_quota"`

	// Grok model mapping policy (admin settings; empty account mapping falls back to these).
	GrokDefaultTextModel           string `json:"grok_default_text_model"`
	GrokCrossClientModelMapEnabled bool   `json:"grok_cross_client_model_map_enabled"`
	GrokDefaultBaseURLMode         string `json:"grok_default_base_url_mode"`

	// Available Channels feature switch (user-facing aggregate view)
	AvailableChannelsEnabled bool `json:"available_channels_enabled"`

	// Model Plaza feature (public group/model pricing showcase)
	ModelPlazaEnabled     bool   `json:"model_plaza_enabled"`
	ModelPlazaRequireAuth bool   `json:"model_plaza_require_auth"`
	ModelPlazaDescription string `json:"model_plaza_description"`

	// 风控中心功能开关
	RiskControlEnabled bool `json:"risk_control_enabled"`

	// cyber 会话屏蔽开关 + TTL
	CyberSessionBlockEnabled    bool `json:"cyber_session_block_enabled"`
	CyberSessionBlockTTLSeconds int  `json:"cyber_session_block_ttl_seconds"`

	// OpenAI fast/flex policy
	OpenAIFastPolicySettings *OpenAIFastPolicySettings `json:"openai_fast_policy_settings,omitempty"`

	// 系统全局默认平台配额（key = platform，nil/缺省 = 不限制）
	DefaultPlatformQuotas map[string]*service.DefaultPlatformQuotaSetting `json:"default_platform_quotas,omitempty"`

	// 系统全局账号自动停调阈值（key = platform，100 = disabled）
	AccountSchedulingThresholds map[string]int `json:"account_scheduling_thresholds,omitempty"`

	// 允许终端用户在用量页查看自己的失败请求
	AllowUserViewErrorRequests bool `json:"allow_user_view_error_requests"`
}

type DefaultSubscriptionSetting struct {
	GroupID      int64 `json:"group_id"`
	ValidityDays int   `json:"validity_days"`
}

type PublicSettings struct {
	RegistrationEmailSuffixWhitelist    []string         `json:"registration_email_suffix_whitelist"`
	RegistrationEmailDomainQuotaEnabled bool             `json:"registration_email_domain_quota_enabled"`
	TotpEnabled                         bool             `json:"totp_enabled"` // TOTP 双因素认证
	PasskeyEnabled                      bool             `json:"passkey_enabled"`
	DocURL                              string           `json:"doc_url"`
	CustomEndpoints                     []CustomEndpoint `json:"custom_endpoints"`
	BackendModeEnabled                  bool             `json:"backend_mode_enabled"`
	Version                             string           `json:"version"`
	// 服务器全局时区（IANA 名称与当前 UTC 偏移，如 "Asia/Shanghai" / "+08:00"）。
	// 高峰时段等按服务器本地时间判定的窗口，前端展示时据此标注，避免用户按浏览器本地时间误读。
	ServerTimezone  string `json:"server_timezone"`
	ServerUTCOffset string `json:"server_utc_offset"`

	ChannelMonitorEnabled                bool   `json:"channel_monitor_enabled"`
	ChannelMonitorMode                   string `json:"channel_monitor_mode"`
	ChannelMonitorDefaultIntervalSeconds int    `json:"channel_monitor_default_interval_seconds"`
	ChannelMonitorHideThroughput         bool   `json:"channel_monitor_hide_throughput"`
	ChannelMonitorShowQuota              bool   `json:"channel_monitor_show_quota"`

	AvailableChannelsEnabled bool `json:"available_channels_enabled"`

	ModelPlazaEnabled     bool `json:"model_plaza_enabled"`
	ModelPlazaRequireAuth bool `json:"model_plaza_require_auth"`

	RiskControlEnabled bool `json:"risk_control_enabled"`

	AllowUserViewErrorRequests bool `json:"allow_user_view_error_requests"`

	// LoginEntryPublic / DefaultHomePath 来自本地配置文件的 web 分组（不可通过后台修改）。
	//
	// 只放"入口是否公开"这个布尔和默认首页路径，绝不放自定义登录路径：这份结构会被
	// 注入进每一个页面的 HTML，也会原样从 /api/v1/settings/public 返回，放进来的东西
	// 等同于公开。
	LoginEntryPublic bool   `json:"login_entry_public"`
	DefaultHomePath  string `json:"default_home_path"`
}

// OverloadCooldownSettings 529过载冷却配置 DTO
type OverloadCooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownMinutes int  `json:"cooldown_minutes"`
}

// RateLimit429CooldownSettings 429默认回避配置 DTO
type RateLimit429CooldownSettings struct {
	Enabled         bool `json:"enabled"`
	CooldownSeconds int  `json:"cooldown_seconds"`
}

// PanelRateLimitSettings 面板 API 限流配置 DTO
type PanelRateLimitSettings struct {
	Enabled     bool `json:"enabled"`
	UserRPM     int  `json:"user_rpm"`
	HeavyRPM    int  `json:"heavy_rpm"`
	ExemptAdmin bool `json:"exempt_admin"`
	PublicIPRPM int  `json:"public_ip_rpm"`
}

// StreamTimeoutSettings 流超时处理配置 DTO
type StreamTimeoutSettings struct {
	Enabled                bool   `json:"enabled"`
	Action                 string `json:"action"`
	TempUnschedMinutes     int    `json:"temp_unsched_minutes"`
	ThresholdCount         int    `json:"threshold_count"`
	ThresholdWindowMinutes int    `json:"threshold_window_minutes"`
}

// RectifierSettings 请求整流器配置 DTO
type RectifierSettings struct {
	Enabled                  bool     `json:"enabled"`
	ThinkingSignatureEnabled bool     `json:"thinking_signature_enabled"`
	ThinkingBudgetEnabled    bool     `json:"thinking_budget_enabled"`
	APIKeySignatureEnabled   bool     `json:"apikey_signature_enabled"`
	APIKeySignaturePatterns  []string `json:"apikey_signature_patterns"`
}

// BetaPolicyRule Beta 策略规则 DTO
type BetaPolicyRule struct {
	BetaToken            string   `json:"beta_token"`
	Action               string   `json:"action"`
	Scope                string   `json:"scope"`
	ErrorMessage         string   `json:"error_message,omitempty"`
	ModelWhitelist       []string `json:"model_whitelist,omitempty"`
	FallbackAction       string   `json:"fallback_action,omitempty"`
	FallbackErrorMessage string   `json:"fallback_error_message,omitempty"`
}

// BetaPolicySettings Beta 策略配置 DTO
type BetaPolicySettings struct {
	Rules []BetaPolicyRule `json:"rules"`
}

// OpenAIFastPolicyRule OpenAI fast/flex 策略规则 DTO
type OpenAIFastPolicyRule struct {
	ServiceTier          string   `json:"service_tier"`
	Action               string   `json:"action"`
	Scope                string   `json:"scope"`
	UserIDs              []int64  `json:"user_ids,omitempty"`
	ErrorMessage         string   `json:"error_message,omitempty"`
	ModelWhitelist       []string `json:"model_whitelist,omitempty"`
	FallbackAction       string   `json:"fallback_action,omitempty"`
	FallbackErrorMessage string   `json:"fallback_error_message,omitempty"`
}

// OpenAIFastPolicySettings OpenAI fast 策略配置 DTO
type OpenAIFastPolicySettings struct {
	Rules []OpenAIFastPolicyRule `json:"rules"`
}

// ParseCustomEndpoints parses a JSON string into a slice of CustomEndpoint.
// Returns empty slice on empty/invalid input.
func ParseCustomEndpoints(raw string) []CustomEndpoint {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []CustomEndpoint{}
	}
	var items []CustomEndpoint
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []CustomEndpoint{}
	}
	return items
}
