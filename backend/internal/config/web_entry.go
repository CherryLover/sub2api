package config

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultHomePathFallback 是 web.default_home_path 的默认值：免登录用量查询页。
const DefaultHomePathFallback = "/key-usage"

// LoginEntryPathMinLength 是自定义登录路径去掉前导 "/" 后的最短长度。
//
// 这不是"安全阈值"，只是挡住 "/x" 这种一眼就能撞上的配置。自定义路径的全部
// 作用是不被猜到，因此实际使用时建议给一段足够长的随机串。
const LoginEntryPathMinLength = 4

// loginEntryPathMaxLength 限制整条路径长度，避免异常长的路径进 HTML/日志。
const loginEntryPathMaxLength = 128

// allowedDefaultHomePaths 是 web.default_home_path 允许的取值。
//
// 只收无需登录即可打开的页面：把需要登录的页面设成默认首页，会让未登录访问
// 陷入「首页 -> 登录跳转 -> 首页」的循环。"/login" 只有在登录入口公开时才允许。
var allowedDefaultHomePaths = map[string]bool{
	"/home":        true,
	"/key-usage":   true,
	"/model-plaza": true,
}

// AllowedDefaultHomePaths 返回默认首页白名单（不含只在登录入口公开时可用的 "/login"）。
// 管理后台的校验与报错文案共用这一份，避免前后端各写一份很快漂移。
func AllowedDefaultHomePaths() []string {
	out := make([]string, 0, len(allowedDefaultHomePaths))
	for path := range allowedDefaultHomePaths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// IsAllowedDefaultHomePath 判断归一化后的路径能否作为默认首页。
//
// loginEntryPublic=false 时 "/login" 不是可达页面，用它当落地页会让未登录访问在
// 「首页 -> 登录跳转 -> 首页」之间无限打转，因此按登录入口的实际状态判定。
func IsAllowedDefaultHomePath(path string, loginEntryPublic bool) bool {
	normalized := NormalizeEntryPath(path)
	if allowedDefaultHomePaths[normalized] {
		return true
	}
	return normalized == "/login" && loginEntryPublic
}

// reservedEntryPaths 是自定义登录路径不能占用的确切路径（既有前端路由与静态资源）。
var reservedEntryPaths = map[string]bool{
	"/":                   true,
	"/home":               true,
	"/login":              true,
	"/register":           true,
	"/email-verify":       true,
	"/forgot-password":    true,
	"/reset-password":     true,
	"/key-usage":          true,
	"/model-plaza":        true,
	"/dashboard":          true,
	"/keys":               true,
	"/usage":              true,
	"/available-channels": true,
	"/profile":            true,
	"/subscriptions":      true,
	"/monitor":            true,
	"/setup":              true,
	"/health":             true,
	"/models":             true,
	"/responses":          true,
	"/favicon.ico":        true,
	"/logo.svg":           true,
	"/robots.txt":         true,
}

// reservedEntryPrefixes 是自定义登录路径不能落在其下的前缀（后端直通前缀 +
// 既有前端路由组 + 静态资源目录）。命中其中任意一个都会让自定义路径要么被后端
// 抢走、要么撞上已有页面。
var reservedEntryPrefixes = []string{
	"/api",
	"/v1",
	"/v1beta",
	"/backend-api",
	"/antigravity",
	"/setup",
	"/responses",
	"/alpha",
	"/images",
	"/videos",
	"/auth",
	"/admin",
	"/legal",
	"/custom",
	"/docs",
	"/assets",
	"/static",
}

// NormalizeEntryPath 去掉尾部斜杠并压掉首尾空白，"/" 保持原样。
// 后端匹配请求路径、前端注册隐藏路由都用同一套归一化规则。
func NormalizeEntryPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "/" {
		return trimmed
	}
	return strings.TrimRight(trimmed, "/")
}

// normalizeAndValidateWeb 归一化并校验 web 分组。
//
// 校验失败一律直接返回错误让进程起不来：登录入口隐藏配置写错时如果静默退化成
// "按公开模式跑"，站长会以为入口已经藏好了，那比启动失败危险得多。
func (c *Config) normalizeAndValidateWeb() error {
	c.Web.LoginEntryPath = NormalizeEntryPath(c.Web.LoginEntryPath)
	c.Web.DefaultHomePath = NormalizeEntryPath(c.Web.DefaultHomePath)

	if c.Web.DefaultHomePath == "" {
		c.Web.DefaultHomePath = DefaultHomePathFallback
	}
	if !IsAllowedDefaultHomePath(c.Web.DefaultHomePath, c.Web.LoginEntryPublic) {
		return fmt.Errorf(
			"web.default_home_path %q is not an allowed landing page (allowed: /home, /key-usage, /model-plaza%s)",
			c.Web.DefaultHomePath,
			loginHomeHint(c.Web.LoginEntryPublic),
		)
	}

	if c.Web.LoginEntryPublic {
		// 公开模式下自定义路径不生效，但仍然校验格式：避免站长写好一条非法路径、
		// 之后把 login_entry_public 翻成 false 时才在生产上炸掉。
		if c.Web.LoginEntryPath != "" {
			if err := validateLoginEntryPath(c.Web.LoginEntryPath); err != nil {
				return err
			}
		}
		return nil
	}

	if c.Web.LoginEntryPath == "" {
		return fmt.Errorf("web.login_entry_path is required when web.login_entry_public is false (set a hard-to-guess path such as \"/j7q2m9x4vk\", or set web.login_entry_public back to true)")
	}
	return validateLoginEntryPath(c.Web.LoginEntryPath)
}

func loginHomeHint(loginPublic bool) string {
	if loginPublic {
		return ", /login"
	}
	return ""
}

// validateLoginEntryPath 校验来自本地配置文件的自定义登录路径。
// 错误信息带上 yaml 键名，让站长知道去改哪一行。
func validateLoginEntryPath(path string) error {
	if err := ValidateLoginEntryPath(path); err != nil {
		return fmt.Errorf("web.login_entry_path: %w", err)
	}
	return nil
}

// ValidateLoginEntryPath 校验自定义登录路径的格式与冲突。
//
// 配置文件与管理后台共用同一套规则：后台能存进数据库的路径，必须也是配置文件
// 认得的路径，否则"后台设一次、配置文件救一次"两条通道会给出不同结论。
// 错误信息里不带 yaml 键名，方便管理后台直接展示给管理员。
func ValidateLoginEntryPath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("login path %q must start with \"/\"", path)
	}
	if len(path) > loginEntryPathMaxLength {
		return fmt.Errorf("login path must be at most %d characters", loginEntryPathMaxLength)
	}
	if len(path)-1 < LoginEntryPathMinLength {
		return fmt.Errorf("login path %q is too short: use at least %d characters after the leading \"/\" (a long random string is what makes it hard to guess)", path, LoginEntryPathMinLength)
	}

	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("login path %q must not contain empty path segments or a trailing slash", path)
		}
		if !isLoginEntrySegment(segment) {
			return fmt.Errorf("login path %q may only contain letters, digits, \"-\", \"_\" and \"~\" in each path segment", path)
		}
	}

	if reservedEntryPaths[path] {
		return fmt.Errorf("login path %q collides with an existing route or static asset", path)
	}
	for _, prefix := range reservedEntryPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return fmt.Errorf("login path %q is not allowed: %q is reserved by the backend or by existing frontend routes", path, prefix)
		}
	}
	return nil
}

func isLoginEntrySegment(segment string) bool {
	for _, r := range segment {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '~':
		default:
			return false
		}
	}
	return true
}

// LoginEntryHidden 表示登录入口是否处于隐藏模式。
//
// 同时要求 LoginEntryPath 非空：没有自定义路径的"隐藏"会把所有人关在门外，
// Validate 已经保证真正的隐藏配置一定带路径，这里再兜一层，让零值 Config
// （单测里常见）保持历史行为——登录入口公开。
func (c *Config) LoginEntryHidden() bool {
	return c != nil && !c.Web.LoginEntryPublic && strings.TrimSpace(c.Web.LoginEntryPath) != ""
}

// ResolvedDefaultHomePath 返回归一化后的默认首页路径，配置缺失时回落到 /key-usage。
func (c *Config) ResolvedDefaultHomePath() string {
	if c == nil || strings.TrimSpace(c.Web.DefaultHomePath) == "" {
		return DefaultHomePathFallback
	}
	return c.Web.DefaultHomePath
}

// WebLoginEntryLockedLocally 表示登录入口（公开与否 + 自定义路径）是否被本地配置锁定。
//
// 锁定时数据库里的值一律忽略，管理后台只读——这就是把自己关在门外之后的破窗通道：
// 往 config.yaml 写一行 web.login_entry_public: true 再重启，入口立刻回到 /login。
//
// 判定条件里除了 load() 记录的显式标记，还带上"LoginEntryPath 非空"：单测和内嵌
// 场景会直接构造 Config 而不过 load()，那时路径本身就是最可靠的显式信号。
func (c *Config) WebLoginEntryLockedLocally() bool {
	return c != nil && (c.Web.LoginEntryConfigured || strings.TrimSpace(c.Web.LoginEntryPath) != "")
}

// WebDefaultHomeLockedLocally 表示默认首页是否被本地配置锁定。
//
// 这里只认 load() 记录的显式标记：normalizeAndValidateWeb 会把空值补成 /key-usage，
// 所以 DefaultHomePath 非空并不能说明站长写过这一行。
func (c *Config) WebDefaultHomeLockedLocally() bool {
	return c != nil && c.Web.DefaultHomePathConfigured
}
