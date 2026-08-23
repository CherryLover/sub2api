package web

import (
	"crypto/subtle"
	"strings"
)

// LoginEntryFlagPlaceholder 是注入进 index.html 的登录入口标记占位符。
//
// 它和 NonceHTMLPlaceholder 一样只存在于「缓存里的那份 HTML」中，出站前才被替换
// 成 "1"（命中自定义登录路径）或 "0"（其它任何页面）。这样做有两个好处：
//   - 共享的 HTMLCache 里永远不会存在带 "1" 的那一份，普通页面不可能拿到登录标记；
//   - "0" 和 "1" 等长，两种响应的字节数完全一致，命中与未命中无法靠 Content-Length
//     或响应耗时区分。
const LoginEntryFlagPlaceholder = "__LOGIN_ENTRY_FLAG__"

// LoginEntry 描述登录入口的布置方式。
//
// Hidden=false 时是历史行为：/login 公开可用，这里的 Path 不参与任何逻辑。
// Hidden=true 时 Path 是自定义登录路径——它只存在于服务端内存和本地配置文件里，
// 不进公开设置接口、不进注入 HTML 的设置 JSON、也不进前端产物。
//
// 需要说清楚的是：隐藏登录路径属于 security through obscurity，它减少的是登录页
// 被扫描器/顺手试探撞见的暴露面，拦不住任何人直接调用 /api/v1/auth/login。
// 抗暴力破解仍然要靠强密码、2FA、IP 限制和限流。
type LoginEntry struct {
	Hidden bool
	Path   string
}

// Enabled 表示是否真的需要走隐藏登录入口逻辑。
func (e LoginEntry) Enabled() bool {
	return e.Hidden && e.Path != ""
}

// NormalizeEntryPath 去掉尾部斜杠，"/" 保持原样。与 config.NormalizeEntryPath 同规则，
// 这里重复一份是为了不让 internal/web 反向依赖 internal/config。
func NormalizeEntryPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "/" {
		return trimmed
	}
	return strings.TrimRight(trimmed, "/")
}

// Matches 判断请求路径是否命中自定义登录路径。
//
// 用 subtle.ConstantTimeCompare 而不是 == 是顺手做掉的一点：长度不同时它直接返回 0，
// 但长度是攻击者自己决定的，本来就不构成信息泄露；等长时逐字节耗时一致，避免把
// "前缀猜对了几个字符" 通过响应耗时透出去。
func (e LoginEntry) Matches(requestPath string) bool {
	if !e.Enabled() {
		return false
	}
	candidate := NormalizeEntryPath(requestPath)
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(e.Path)) == 1
}
