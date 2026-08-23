/**
 * 登录入口 / 默认首页解析。
 *
 * 站长可以在后端本地配置文件的 `web` 分组里把登录入口藏起来：`/login` 不再可用，
 * 登录页只在一条自定义路径上渲染。这里是前端这一侧的全部决策点。
 *
 * ## 自定义路径为什么不会泄漏
 *
 * 前端产物是静态文件，任何写进 bundle 或写进公开接口的东西都等于公开。因此这条
 * 路径在前端只有一个来源：**当前这次页面加载的 URL 本身**。后端只在请求路径正好
 * 命中自定义路径时，才往那一次的 index.html 里注入 `window.__LOGIN_ENTRY__=1`；
 * 其它任何页面拿到的都是 `=0`。也就是说浏览器只有"自己已经走到过那条路径"时才知道
 * 它，不知道的人无论看源码、拉 `/api/v1/settings/public` 还是翻 JS 都拿不到。
 *
 * 捕获到之后写进 sessionStorage（按标签页隔离），是为了让 OAuth 跳转回来这种
 * 「离开页面再回来」的流程还能找回登录入口。sessionStorage 只有同源 JS 能读，
 * 对不知道路径的人不构成任何泄漏。
 *
 * ## 诚实说明
 *
 * 隐藏登录路径是 security through obscurity：它减少的是登录页被扫描器和顺手试探
 * 撞见的暴露面，**并不能阻止**任何人直接调用 `/api/v1/auth/login` 之类的接口。
 * 真正扛暴力破解/撞库的是强密码、2FA、IP 限制和限流。
 */

import type { PublicSettings } from '@/types'

/** 后端 `web.default_home_path` 的默认值，与 config.DefaultHomePathFallback 保持一致。 */
export const FALLBACK_DEFAULT_HOME_PATH = '/key-usage'

/** `/login` 的静态路径（登录入口公开时使用）。 */
export const PUBLIC_LOGIN_PATH = '/login'

/**
 * 允许作为默认首页的路径。
 *
 * 只收免登录页面：把需要登录的页面设成默认首页，会让未登录访问在
 * 「默认首页 -> 未登录跳转 -> 默认首页」之间打转。后端 Validate 已经用同一份
 * 白名单挡过一次，这里再挡一次，避免后端换版本/配置漂移时前端跟着转圈。
 */
const ALLOWED_DEFAULT_HOME_PATHS = new Set([
  '/home',
  '/key-usage',
  '/model-plaza',
  PUBLIC_LOGIN_PATH,
])

const ENTRY_SESSION_KEY = 'sub2api:login-entry-path'

/** 归一化路径：去掉尾部斜杠（`/` 除外），与后端 NormalizeEntryPath 同规则。 */
function normalizePath(raw: string): string {
  const trimmed = (raw || '').trim()
  if (trimmed === '' || trimmed === '/') return trimmed
  return trimmed.replace(/\/+$/, '') || '/'
}

function readStoredEntryPath(): string | null {
  try {
    const stored = window.sessionStorage.getItem(ENTRY_SESSION_KEY)
    return stored ? normalizePath(stored) : null
  } catch {
    // 隐私模式 / 禁用存储：只是拿不到登录入口，不影响其它页面。
    return null
  }
}

function storeEntryPath(path: string): void {
  try {
    window.sessionStorage.setItem(ENTRY_SESSION_KEY, path)
  } catch {
    // 存不下就算了，本次页面加载内仍然可用。
  }
}

let capturedEntryPath: string | null | undefined

/**
 * 捕获本标签页已知的隐藏登录路径。
 *
 * 必须在路由发生第一次导航之前调用（router/index.ts 在模块顶层调用了一次），
 * 否则 `window.location.pathname` 已经不是入口 URL 了。
 */
export function captureLoginEntryPath(): string | null {
  if (capturedEntryPath !== undefined) return capturedEntryPath
  if (typeof window === 'undefined') {
    capturedEntryPath = null
    return capturedEntryPath
  }

  if (window.__LOGIN_ENTRY__ === 1) {
    const path = normalizePath(window.location.pathname)
    if (path && path !== '/') {
      storeEntryPath(path)
      capturedEntryPath = path
      return capturedEntryPath
    }
  }

  capturedEntryPath = readStoredEntryPath()
  return capturedEntryPath
}

/** 仅供测试：清掉模块级缓存。 */
export function resetLoginEntryCacheForTests(): void {
  capturedEntryPath = undefined
}

function injectedSettings(): PublicSettings | null {
  if (typeof window === 'undefined') return null
  return window.__APP_CONFIG__ ?? null
}

function effectiveSettings(settings?: PublicSettings | null): PublicSettings | null {
  return settings === undefined ? injectedSettings() : settings
}

/**
 * 登录入口是否处于隐藏模式。
 *
 * 只有明确读到 `login_entry_public === false` 才算隐藏。设置没加载到时按公开处理
 * （fail-open）：这里 fail-closed 的代价是"配置没读到就没人能登录"，而 fail-open
 * 的代价只是"入口没藏住"——后者可以靠 /login 的守卫在设置到位后立刻纠正，前者会
 * 直接把站长关在门外。
 */
export function isLoginEntryHidden(settings?: PublicSettings | null): boolean {
  return effectiveSettings(settings)?.login_entry_public === false
}

/** 访问 `/` 时应该落到的页面。 */
export function resolveDefaultHomePath(settings?: PublicSettings | null): string {
  const configured = normalizePath(effectiveSettings(settings)?.default_home_path ?? '')
  if (configured && ALLOWED_DEFAULT_HOME_PATHS.has(configured)) {
    // 隐藏模式下 /login 不是可用页面，不能当落地页。
    if (configured === PUBLIC_LOGIN_PATH && isLoginEntryHidden(settings)) {
      return FALLBACK_DEFAULT_HOME_PATH
    }
    return configured
  }
  return FALLBACK_DEFAULT_HOME_PATH
}

/**
 * 登录页在当前这次会话里的可用路径；返回 null 表示"这个浏览器不知道入口在哪"，
 * 调用方应该直接不渲染登录入口。
 */
export function resolveLoginPath(settings?: PublicSettings | null): string | null {
  if (!isLoginEntryHidden(settings)) return PUBLIC_LOGIN_PATH
  return captureLoginEntryPath()
}

/** 是否应该展示登录入口（首页按钮、公开页的"去登录"链接等）。 */
export function isLoginEntryVisible(settings?: PublicSettings | null): boolean {
  return resolveLoginPath(settings) !== null
}

/**
 * 未登录用户被挡下来时应该去哪。
 *
 * 公开模式：还是 `/login`，并带上 redirect 让登录后能回到原页面。
 * 隐藏模式：只有本标签页已经知道入口时才回到登录页；否则送去默认首页——绝不能
 * 因为"随便访问一个受保护 URL"就把入口透出去。
 */
export function resolveUnauthenticatedTarget(
  settings?: PublicSettings | null,
  redirectFullPath?: string
): { path: string; query?: Record<string, string> } {
  const loginPath = resolveLoginPath(settings)
  if (!loginPath) {
    return { path: resolveDefaultHomePath(settings) }
  }
  return redirectFullPath ? { path: loginPath, query: { redirect: redirectFullPath } } : { path: loginPath }
}

/**
 * 隐藏模式下需要跟着登录页一起藏起来的"入口页"路由名。
 *
 * `/register`（自助注册）和 `/forgot-password`（匿名触发重置邮件）都是登录之外的
 * 另一条对外入口：留着它们既让"藏起登录入口"失去意义，页面上的"已有账号？登录"
 * 还会指向一条隐藏模式下不存在的 /login。因此这两个页面只在本标签页已经走过隐藏
 * 登录入口时才可达——从隐藏登录页点过去正常工作，直接开一个新标签页访问则不可达。
 *
 * `/reset-password` 和 `/email-verify` 不在此列：它们是从邮件里点进来的，没有登录
 * 入口上下文，挡掉会直接把这两条流程做死。OAuth 回调 `/auth/*` 也不在此列。
 */
const HIDDEN_MODE_GATED_ROUTE_NAMES = new Set(['Register', 'ForgotPassword'])

/** 需要在隐藏模式下做入口判定的路由（静态 /login、被门禁的注册/找回、动态入口）。 */
export function isEntryRoute(route: {
  name?: string | symbol | null
  meta?: { loginEntry?: boolean }
}): boolean {
  return (
    route.meta?.loginEntry === true ||
    route.name === 'Login' ||
    HIDDEN_MODE_GATED_ROUTE_NAMES.has(String(route.name ?? ''))
  )
}

/**
 * 隐藏模式下该不该挡掉这次导航。
 *
 * - 动态注册的隐藏入口（meta.loginEntry）：永远放行，它就是登录页本身。
 * - 静态 `/login`：隐藏模式下永远挡掉——"隐藏时 /login 不再可用"。
 * - 注册 / 找回密码：只有本标签页已经知道隐藏入口时才放行。
 */
export function isEntryRouteBlocked(
  route: { name?: string | symbol | null; meta?: { loginEntry?: boolean } },
  settings?: PublicSettings | null
): boolean {
  if (route.meta?.loginEntry === true) return false
  if (!isLoginEntryHidden(settings)) return false
  if (route.name === 'Login') return true
  if (HIDDEN_MODE_GATED_ROUTE_NAMES.has(String(route.name ?? ''))) {
    return captureLoginEntryPath() === null
  }
  return false
}

/**
 * 需要整页跳转（axios 401 兜底、setup 向导结束等）时的目标地址。
 */
export function resolveLoginHref(settings?: PublicSettings | null): string {
  return resolveLoginPath(settings) ?? resolveDefaultHomePath(settings)
}
