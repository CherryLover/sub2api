package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// 登录入口 / 默认首页的三层优先级解析。
//
//	本地配置文件（显式设置时） > 数据库（管理后台） > 内置默认值
//
// 为什么要留最上面那一层：这三项现在可以在后台改，而改错就是"登录页再也打不开、
// 服务却照常运行"。配置文件是破窗通道——管理员把自己关在门外时，往 config.yaml 写
// 一行再重启即可夺回入口，数据库里的值被完全忽略。config.WebLoginEntryLockedLocally /
// WebDefaultHomeLockedLocally 负责判定"显式设置过"。
//
// 关于自定义路径的一条红线：LoginEntryPath 只允许出现在服务端内存、数据库和
// 管理端（需要管理员鉴权）的响应里。它绝不能进 PublicSettings、绝不能进注入每个
// 页面的设置 JSON、也绝不能进前端产物。

const (
	webEntryCacheTTL   = 30 * time.Second
	webEntryErrorTTL   = 5 * time.Second
	webEntryDBTimeout  = 5 * time.Second
	webEntrySFCacheKey = "web_entry_settings"
)

// WebEntrySettings 是登录入口 / 默认首页三层合并之后的最终结果。
type WebEntrySettings struct {
	// LoginEntryPublic 为 false 表示隐藏模式：/login 不再可用，登录页只在
	// LoginEntryPath 上渲染。
	LoginEntryPublic bool
	// LoginEntryPath 自定义登录路径。仅隐藏模式下有意义，且只允许流向管理端。
	LoginEntryPath string
	// DefaultHomePath 访问 "/" 时落到的页面。
	DefaultHomePath string
	// LoginEntryLockedByConfig / DefaultHomeLockedByConfig 表示该项当前由本地
	// 配置文件锁定，管理后台只读。界面据此禁用输入并说明原因，避免管理员改了
	// 半天没反应还以为是 bug。
	LoginEntryLockedByConfig  bool
	DefaultHomeLockedByConfig bool
}

// LoginEntryHidden 表示登录入口是否真的处于可用的隐藏模式。
// 隐藏但没有路径 = 谁都进不来，这里一律按公开处理（解析阶段已经兜过一次）。
func (w WebEntrySettings) LoginEntryHidden() bool {
	return !w.LoginEntryPublic && strings.TrimSpace(w.LoginEntryPath) != ""
}

type cachedWebEntrySettings struct {
	settings  WebEntrySettings
	expiresAt int64 // unix nano
}

// defaultWebEntrySettings 是三层都缺席时的内置默认值：登录入口公开、"/" 落到免登录用量页。
func defaultWebEntrySettings() WebEntrySettings {
	return WebEntrySettings{
		LoginEntryPublic: true,
		DefaultHomePath:  config.DefaultHomePathFallback,
	}
}

// ResolveWebEntry 返回合并后的登录入口配置（进程内缓存，30s TTL）。
//
// 每个 index.html 请求都要用它判断"这次请求是不是命中了隐藏登录路径"，所以绝不能
// 每次访问数据库；后台保存时会立即刷新本节点缓存，多节点部署最迟 30s 内一致。
func (s *SettingService) ResolveWebEntry(ctx context.Context) WebEntrySettings {
	if s == nil {
		return defaultWebEntrySettings()
	}
	if cached, ok := s.webEntryCache.Load().(*cachedWebEntrySettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.settings
		}
	}

	result, _, _ := s.webEntrySF.Do(webEntrySFCacheKey, func() (any, error) {
		if cached, ok := s.webEntryCache.Load().(*cachedWebEntrySettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.settings, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		// 独立 context：断开请求取消链，避免客户端断连污染缓存。
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), webEntryDBTimeout)
		defer cancel()

		stored := map[string]string{}
		if s.settingRepo != nil && !s.webEntryFullyLockedLocally() {
			values, err := s.settingRepo.GetMultiple(dbCtx, []string{
				SettingKeyWebLoginEntryPublic,
				SettingKeyWebLoginEntryPath,
				SettingKeyWebDefaultHomePath,
			})
			if err != nil {
				slog.Warn("failed to read web entry settings; keeping the last known layout", "error", err)
				// 数据库读不到时保留最近一次已知布置；连这个都没有就按"数据库是空的"
				// 合并一次——那条路径的结论是登录入口公开，数据库故障不该顺带把登录页藏起来。
				fallback := s.mergeWebEntrySettings(nil)
				if prior, ok := s.webEntryCache.Load().(*cachedWebEntrySettings); ok && prior != nil {
					fallback = prior.settings
				}
				s.storeWebEntryCache(fallback, webEntryErrorTTL)
				return fallback, nil
			}
			stored = values
		}

		merged := s.mergeWebEntrySettings(stored)
		s.storeWebEntryCache(merged, webEntryCacheTTL)
		return merged, nil
	})
	if settings, ok := result.(WebEntrySettings); ok {
		return settings
	}
	return defaultWebEntrySettings()
}

// webEntryFullyLockedLocally 表示两项都被本地配置锁定，这时连读数据库都不必。
func (s *SettingService) webEntryFullyLockedLocally() bool {
	return s.cfg.WebLoginEntryLockedLocally() && s.cfg.WebDefaultHomeLockedLocally()
}

// refreshWebEntryCacheFromSettings 用刚落库的这份设置重建缓存。
//
// 刻意不回读数据库：保存路径上再发一次查询既没必要，也会让"设置保存"依赖一次额外的
// 数据库往返。settings 里的三项就是刚写进去的值（被本地配置锁定的项由 handler 填成
// 生效值），再过一遍 mergeWebEntrySettings 即可拿到与解析路径完全一致的结论。
func (s *SettingService) refreshWebEntryCacheFromSettings(settings *SystemSettings) {
	if s == nil || settings == nil {
		return
	}
	s.storeWebEntryCache(s.mergeWebEntrySettings(map[string]string{
		SettingKeyWebLoginEntryPublic: strconv.FormatBool(settings.LoginEntryPublic),
		SettingKeyWebLoginEntryPath:   settings.LoginEntryPath,
		SettingKeyWebDefaultHomePath:  settings.DefaultHomePath,
	}), webEntryCacheTTL)
}

// mergeWebEntrySettings 执行三层合并。stored 为数据库里已存的原始值（可为 nil）。
func (s *SettingService) mergeWebEntrySettings(stored map[string]string) WebEntrySettings {
	result := defaultWebEntrySettings()
	cfg := s.cfg

	// —— 登录入口 ——
	if cfg.WebLoginEntryLockedLocally() {
		result.LoginEntryLockedByConfig = true
		result.LoginEntryPublic = !cfg.LoginEntryHidden()
		if cfg.LoginEntryHidden() {
			result.LoginEntryPath = cfg.Web.LoginEntryPath
		}
	} else {
		// 数据库层：只有显式存了 "false" 才算隐藏，缺失/空/脏值一律按公开，保持历史行为。
		public := strings.TrimSpace(stored[SettingKeyWebLoginEntryPublic]) != "false"
		path := config.NormalizeEntryPath(stored[SettingKeyWebLoginEntryPath])
		if !public {
			// fail-open：数据库里存着"隐藏但路径非法/为空"时按公开处理。
			// 保存路径上已经挡过一次，这里是最后一道兜底——宁可入口没藏住，也不能
			// 因为一条坏数据把所有人（包括站长）关在门外。
			if path == "" || config.ValidateLoginEntryPath(path) != nil {
				slog.Error("stored login entry is hidden but its custom path is unusable; serving the public login entry instead",
					"has_path", path != "")
				public = true
				path = ""
			}
		}
		result.LoginEntryPublic = public
		if !public {
			result.LoginEntryPath = path
		}
	}

	// —— 默认首页 ——
	home := ""
	if cfg.WebDefaultHomeLockedLocally() {
		result.DefaultHomeLockedByConfig = true
		home = cfg.ResolvedDefaultHomePath()
	} else {
		home = config.NormalizeEntryPath(stored[SettingKeyWebDefaultHomePath])
	}
	if home == "" {
		home = config.DefaultHomePathFallback
	}
	// 白名单 + 死循环防护：登录入口隐藏时 "/login" 不是可达页面，用它当落地页会让
	// 未登录访问在「首页 -> 登录跳转 -> 首页」之间无限打转。
	if !config.IsAllowedDefaultHomePath(home, result.LoginEntryPublic) {
		home = config.DefaultHomePathFallback
	}
	result.DefaultHomePath = home
	return result
}

func (s *SettingService) storeWebEntryCache(settings WebEntrySettings, ttl time.Duration) {
	s.webEntryCache.Store(&cachedWebEntrySettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}

// WebEntryValidationError 是保存前校验失败的原因，供管理端原样回显给管理员。
type WebEntryValidationError struct {
	// Field 是出问题的字段（login_entry_path / default_home_path / login_entry_public）。
	Field   string
	Message string
}

func (e *WebEntryValidationError) Error() string { return e.Message }

// NormalizeAndValidateWebEntryInput 校验并归一化管理后台提交的三项。
//
// 保存前一定要过这一关：把"隐藏但路径为空/非法"写进数据库，等于按几下鼠标就让
// 登录页永久打不开。非法值一律拒绝，并把原因说清楚。
func NormalizeAndValidateWebEntryInput(loginEntryPublic bool, loginEntryPath, defaultHomePath string) (string, string, error) {
	path := config.NormalizeEntryPath(loginEntryPath)
	home := config.NormalizeEntryPath(defaultHomePath)

	if path != "" {
		// 公开模式下也校验：避免先存一条非法路径，之后翻成隐藏模式时才发现进不去。
		if err := config.ValidateLoginEntryPath(path); err != nil {
			return "", "", &WebEntryValidationError{Field: "login_entry_path", Message: err.Error()}
		}
	}
	if !loginEntryPublic && path == "" {
		return "", "", &WebEntryValidationError{
			Field:   "login_entry_path",
			Message: "a custom login path is required before the login entry can be hidden (use a long random path such as \"/j7q2m9x4vk3p\")",
		}
	}

	if home == "" {
		home = config.DefaultHomePathFallback
	}
	if !config.IsAllowedDefaultHomePath(home, loginEntryPublic) {
		allowed := strings.Join(config.AllowedDefaultHomePaths(), ", ")
		if loginEntryPublic {
			allowed += ", /login"
		}
		return "", "", &WebEntryValidationError{
			Field:   "default_home_path",
			Message: "default home page " + home + " is not an allowed landing page (allowed: " + allowed + ")",
		}
	}
	return path, home, nil
}

// NormalizeWebEntryPath 暴露给 handler 层的路径归一化（去尾斜杠、压首尾空白）。
// 后端匹配请求路径、前端注册隐藏路由、后台比对"值有没有变"都用同一套规则。
func NormalizeWebEntryPath(raw string) string {
	return config.NormalizeEntryPath(raw)
}
