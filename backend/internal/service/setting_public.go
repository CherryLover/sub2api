package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// GetPublicSettings 获取公开设置（无需登录）
func (s *SettingService) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	keys := []string{
		SettingKeyRegistrationEmailSuffixWhitelist,
		SettingKeyRegistrationEmailDomainQuotaEnabled,
		SettingKeyTotpEnabled,
		SettingKeyPasskeyEnabled,
		SettingKeyAPIKeyACLTrustForwardedIP,
		SettingKeyDocURL,
		SettingKeyCustomEndpoints,
		SettingKeyBackendModeEnabled,
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorHideThroughput,
		SettingKeyAvailableChannelsEnabled,
		SettingKeyRiskControlEnabled,
		SettingKeyAllowUserViewErrorRequests,
	}

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
	}

	// 登录入口走独立的三层解析（本地配置 > 数据库 > 默认），刻意不把
	// web_login_entry_path 混进上面那张 settings map：这份 map 喂的是公开 payload，
	// 少一次共处一室就少一次"哪天被顺手 range 出来"的机会。
	webEntry := s.ResolveWebEntry(ctx)

	registrationEmailSuffixWhitelist := ParseRegistrationEmailSuffixWhitelist(
		settings[SettingKeyRegistrationEmailSuffixWhitelist],
	)

	return &PublicSettings{
		RegistrationEmailSuffixWhitelist:    registrationEmailSuffixWhitelist,
		RegistrationEmailDomainQuotaEnabled: settings[SettingKeyRegistrationEmailDomainQuotaEnabled] == "true",
		TotpEnabled:                         settings[SettingKeyTotpEnabled] == "true",
		PasskeyEnabled:                      s.passkeyConfigured() && s.passkeySettingEnabled(settings),
		DocURL:                              settings[SettingKeyDocURL],
		CustomEndpoints:                     settings[SettingKeyCustomEndpoints],
		BackendModeEnabled:                  settings[SettingKeyBackendModeEnabled] == "true",

		ChannelMonitorEnabled:        !isFalseSettingValue(settings[SettingKeyChannelMonitorEnabled]),
		ChannelMonitorHideThroughput: !isFalseSettingValue(settings[SettingKeyChannelMonitorHideThroughput]),

		AvailableChannelsEnabled: settings[SettingKeyAvailableChannelsEnabled] == "true",

		RiskControlEnabled: settings[SettingKeyRiskControlEnabled] == "true",

		AllowUserViewErrorRequests: settings[SettingKeyAllowUserViewErrorRequests] == "true",

		// 只放"入口是否公开"和默认首页，绝不放 webEntry.LoginEntryPath。
		LoginEntryPublic: !webEntry.LoginEntryHidden(),
		DefaultHomePath:  webEntry.DefaultHomePath,
	}, nil
}

// ChannelMonitorRuntime is the lightweight view of the channel monitor feature
// consumed by the V2 aggregator, the V2 route guards, and user-facing handlers.
type ChannelMonitorRuntime struct {
	Enabled bool
	// HideThroughput: when true, user-facing V2 APIs omit RPM/TPM scale signals.
	HideThroughput bool
}

// PassiveAggregationAllowed reports whether V2 passive aggregation may run.
// Only channel_monitor_enabled gates it: V1 active probing is gone, so there is
// no longer a mode setting selecting between implementations.
func (r ChannelMonitorRuntime) PassiveAggregationAllowed() bool {
	return r.Enabled
}

// GetChannelMonitorRuntime reads the channel monitor feature flags directly from
// the settings store. Fail-open on error: Enabled=true with throughput hidden.
func (s *SettingService) GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntime {
	if s == nil || s.settingRepo == nil {
		return ChannelMonitorRuntime{Enabled: true, HideThroughput: true}
	}
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorHideThroughput,
	})
	if err != nil {
		return ChannelMonitorRuntime{Enabled: true, HideThroughput: true}
	}
	return ChannelMonitorRuntime{
		Enabled:        !isFalseSettingValue(vals[SettingKeyChannelMonitorEnabled]),
		HideThroughput: !isFalseSettingValue(vals[SettingKeyChannelMonitorHideThroughput]),
	}
}


// AvailableChannelsRuntime is the lightweight view of the available-channels feature
// switch consumed by the user-facing handler.
type AvailableChannelsRuntime struct {
	Enabled bool
}

// GetAvailableChannelsRuntime reads the available-channels feature switch directly
// from the settings store. Fail-closed: on error returns Enabled=false, matching
// the opt-in default (unknown ↔ disabled).
func (s *SettingService) GetAvailableChannelsRuntime(ctx context.Context) AvailableChannelsRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAvailableChannelsEnabled})
	if err != nil {
		return AvailableChannelsRuntime{Enabled: false}
	}
	return AvailableChannelsRuntime{
		Enabled: vals[SettingKeyAvailableChannelsEnabled] == "true",
	}
}

// IsUserErrorViewAllowed reads the user-facing error-requests visibility switch
// directly from the settings store. Fail-closed: on error returns false (opt-in default).
func (s *SettingService) IsUserErrorViewAllowed(ctx context.Context) bool {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAllowUserViewErrorRequests})
	if err != nil {
		slog.Warn("failed to get allow_user_view_error_requests setting, defaulting to false", "error", err)
		return false
	}
	return vals[SettingKeyAllowUserViewErrorRequests] == "true"
}

// PublicSettingsInjectionPayload is the JSON shape embedded into HTML as
// `window.__APP_CONFIG__` so the frontend can hydrate feature flags & site
// config before the first XHR finishes.
//
// INVARIANT: every `json` tag here MUST also exist on handler/dto.PublicSettings.
// If you forget a feature-flag field here, the frontend's
// `cachedPublicSettings.xxx_enabled` will be `undefined` on refresh until the
// async `/api/v1/settings/public` call returns — which causes opt-in menus
// (strict `=== true`) to flicker off/on. See
// frontend/src/utils/featureFlags.ts for the matching registry.
//
// A unit test diffs this struct's JSON keys against dto.PublicSettings to catch
// drift automatically (see setting_service_injection_test.go).
type PublicSettingsInjectionPayload struct {
	RegistrationEmailSuffixWhitelist    []string        `json:"registration_email_suffix_whitelist"`
	RegistrationEmailDomainQuotaEnabled bool            `json:"registration_email_domain_quota_enabled"`
	TotpEnabled                         bool            `json:"totp_enabled"`
	PasskeyEnabled                      bool            `json:"passkey_enabled"`
	DocURL                              string          `json:"doc_url"`
	CustomEndpoints                     json.RawMessage `json:"custom_endpoints"`
	BackendModeEnabled                  bool            `json:"backend_mode_enabled"`
	Version                             string          `json:"version"`
	// 服务器全局时区（IANA 名称与当前 UTC 偏移），高峰时段等服务端本地时间窗口的展示标注用
	ServerTimezone  string `json:"server_timezone"`
	ServerUTCOffset string `json:"server_utc_offset"`

	// Feature flags — MUST match the opt-in/opt-out registry in
	// frontend/src/utils/featureFlags.ts. Missing a field here is the bug
	// that hid the "可用渠道" menu on page refresh.
	ChannelMonitorEnabled bool `json:"channel_monitor_enabled"`
	// ChannelMonitorHideThroughput is public so the user UI can hide RPM/TPM
	// without waiting for API redaction alone (defense in depth).
	ChannelMonitorHideThroughput bool `json:"channel_monitor_hide_throughput"`
	AvailableChannelsEnabled     bool `json:"available_channels_enabled"`
	RiskControlEnabled           bool `json:"risk_control_enabled"`
	AllowUserViewErrorRequests   bool `json:"allow_user_view_error_requests"`

	// LoginEntryPublic / DefaultHomePath 来自本地配置文件的 web 分组（不可通过后台修改）。
	//
	// 只放"入口是否公开"这个布尔和默认首页路径，绝不放自定义登录路径：这份结构会被
	// 注入进每一个页面的 HTML，也会原样从 /api/v1/settings/public 返回，放进来的东西
	// 等同于公开。
	LoginEntryPublic bool   `json:"login_entry_public"`
	DefaultHomePath  string `json:"default_home_path"`
}

// GetPublicSettingsForInjection returns public settings in a format suitable for HTML injection.
// This implements the web.PublicSettingsProvider interface.
func (s *SettingService) GetPublicSettingsForInjection(ctx context.Context) (any, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
	}

	return &PublicSettingsInjectionPayload{
		RegistrationEmailSuffixWhitelist:    settings.RegistrationEmailSuffixWhitelist,
		RegistrationEmailDomainQuotaEnabled: settings.RegistrationEmailDomainQuotaEnabled,
		TotpEnabled:                         settings.TotpEnabled,
		PasskeyEnabled:                      settings.PasskeyEnabled,
		DocURL:                              settings.DocURL,
		CustomEndpoints:                     safeRawJSONArray(settings.CustomEndpoints),
		BackendModeEnabled:                  settings.BackendModeEnabled,
		Version:                             s.version,
		ServerTimezone:                      timezone.Name(),
		ServerUTCOffset:                     timezone.UTCOffset(),

		ChannelMonitorEnabled:        settings.ChannelMonitorEnabled,
		ChannelMonitorHideThroughput: settings.ChannelMonitorHideThroughput,
		AvailableChannelsEnabled:     settings.AvailableChannelsEnabled,
		LoginEntryPublic:             settings.LoginEntryPublic,
		DefaultHomePath:              settings.DefaultHomePath,
		RiskControlEnabled:           settings.RiskControlEnabled,
		AllowUserViewErrorRequests:   settings.AllowUserViewErrorRequests,
	}, nil
}

// safeRawJSONArray returns raw as json.RawMessage if it's valid JSON, otherwise "[]".
func safeRawJSONArray(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return json.RawMessage("[]")
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return json.RawMessage("[]")
}
