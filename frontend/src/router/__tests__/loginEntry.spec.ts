import { beforeEach, describe, expect, it } from 'vitest'
import type { PublicSettings } from '@/types'

import {
  FALLBACK_DEFAULT_HOME_PATH,
  captureLoginEntryPath,
  isEntryRoute,
  isEntryRouteBlocked,
  isLoginEntryHidden,
  isLoginEntryVisible,
  resolveDefaultHomePath,
  resolveLoginHref,
  resolveLoginPath,
  resolveUnauthenticatedTarget,
  resetLoginEntryCacheForTests,
} from '../loginEntry'

const HIDDEN_PATH = '/j7q2m9x4vk3p'

function settings(overrides: Partial<PublicSettings> = {}): PublicSettings {
  return overrides as PublicSettings
}

function landOn(path: string, marker?: number) {
  window.history.replaceState({}, '', path)
  if (marker === undefined) {
    delete window.__LOGIN_ENTRY__
  } else {
    window.__LOGIN_ENTRY__ = marker
  }
  resetLoginEntryCacheForTests()
}

describe('loginEntry', () => {
  beforeEach(() => {
    sessionStorage.clear()
    delete window.__APP_CONFIG__
    landOn('/home')
  })

  describe('公开模式（默认，回归）', () => {
    it('设置缺失时按公开处理，登录入口仍是 /login', () => {
      expect(isLoginEntryHidden(undefined)).toBe(false)
      expect(isLoginEntryHidden(null)).toBe(false)
      expect(isLoginEntryHidden(settings())).toBe(false)
      expect(resolveLoginPath(null)).toBe('/login')
      expect(isLoginEntryVisible(null)).toBe(true)
    })

    it('login_entry_public 为 true 时行为不变', () => {
      const s = settings({ login_entry_public: true })
      expect(resolveLoginPath(s)).toBe('/login')
      expect(resolveUnauthenticatedTarget(s, '/dashboard')).toEqual({
        path: '/login',
        query: { redirect: '/dashboard' },
      })
      expect(resolveLoginHref(s)).toBe('/login')
    })

    it('/login 不会被挡', () => {
      const s = settings({ login_entry_public: true })
      expect(isEntryRouteBlocked({ name: 'Login' }, s)).toBe(false)
    })
  })

  describe('默认首页', () => {
    it('默认落到 /key-usage', () => {
      expect(resolveDefaultHomePath(null)).toBe('/key-usage')
      expect(FALLBACK_DEFAULT_HOME_PATH).toBe('/key-usage')
    })

    it('采用配置里的合法页面（含尾斜杠归一化）', () => {
      expect(resolveDefaultHomePath(settings({ default_home_path: '/home' }))).toBe('/home')
      expect(resolveDefaultHomePath(settings({ default_home_path: '/key-usage/' }))).toBe('/key-usage')
    })

    it('拒绝需要登录的页面，避免未登录访问打转', () => {
      expect(resolveDefaultHomePath(settings({ default_home_path: '/dashboard' }))).toBe('/key-usage')
      // 模型广场已删除：曾经的合法落地页现在必须被拒
      expect(resolveDefaultHomePath(settings({ default_home_path: '/model-plaza' }))).toBe('/key-usage')
      expect(resolveDefaultHomePath(settings({ default_home_path: '/admin/dashboard' }))).toBe('/key-usage')
      expect(resolveDefaultHomePath(settings({ default_home_path: 'nonsense' }))).toBe('/key-usage')
    })

    it('隐藏模式下 /login 不能当默认首页', () => {
      const s = settings({ login_entry_public: false, default_home_path: '/login' })
      expect(resolveDefaultHomePath(s)).toBe('/key-usage')
    })
  })

  describe('隐藏模式：不知道路径的访问者', () => {
    const hidden = settings({ login_entry_public: false, default_home_path: '/key-usage' })

    it('没有标记时拿不到任何登录路径', () => {
      landOn('/home')
      expect(captureLoginEntryPath()).toBeNull()
      expect(resolveLoginPath(hidden)).toBeNull()
      expect(isLoginEntryVisible(hidden)).toBe(false)
    })

    it('未登录访问受保护页被送去默认首页，而不是登录页', () => {
      landOn('/home')
      expect(resolveUnauthenticatedTarget(hidden, '/dashboard')).toEqual({ path: '/key-usage' })
      // 目标里既没有登录路径，也没有 redirect 参数可以顺藤摸瓜
      expect(JSON.stringify(resolveUnauthenticatedTarget(hidden, '/dashboard'))).not.toContain(HIDDEN_PATH)
      expect(resolveLoginHref(hidden)).toBe('/key-usage')
    })

    it('/login 被挡', () => {
      landOn('/home')
      expect(isEntryRouteBlocked({ name: 'Login' }, hidden)).toBe(true)
    })

    it('普通页面与 OAuth 回调不受影响', () => {
      landOn('/home')
      expect(isEntryRoute({ name: 'OAuthCallback' })).toBe(false)
      expect(isEntryRouteBlocked({ name: 'KeyUsage' }, hidden)).toBe(false)
      expect(isEntryRouteBlocked({ name: 'LegalDocument' }, hidden)).toBe(false)
      expect(isEntryRouteBlocked({ name: 'Setup' }, hidden)).toBe(false)
    })
  })

  describe('隐藏模式：命中自定义路径的这一次加载', () => {
    const hidden = settings({ login_entry_public: false, default_home_path: '/key-usage' })

    it('从当前 URL 捕获入口路径（而不是从任何配置里读）', () => {
      landOn(HIDDEN_PATH, 1)
      expect(captureLoginEntryPath()).toBe(HIDDEN_PATH)
      expect(resolveLoginPath(hidden)).toBe(HIDDEN_PATH)
      expect(isLoginEntryVisible(hidden)).toBe(true)
    })

    it('尾斜杠被归一化', () => {
      landOn(`${HIDDEN_PATH}/`, 1)
      expect(captureLoginEntryPath()).toBe(HIDDEN_PATH)
    })

    it('标记为 0 的普通页面不会被当成入口', () => {
      landOn('/home', 0)
      expect(captureLoginEntryPath()).toBeNull()
      expect(resolveLoginPath(hidden)).toBeNull()
    })

    it('登录后的 redirect 参数照常工作', () => {
      landOn(HIDDEN_PATH, 1)
      expect(resolveUnauthenticatedTarget(hidden, '/dashboard')).toEqual({
        path: HIDDEN_PATH,
        query: { redirect: '/dashboard' },
      })
    })

    it('动态注册的入口路由永远不会被守卫挡掉；静态 /login 依然被挡', () => {
      landOn(HIDDEN_PATH, 1)
      expect(isEntryRoute({ name: 'LoginEntry', meta: { loginEntry: true } })).toBe(true)
      expect(isEntryRouteBlocked({ name: 'LoginEntry', meta: { loginEntry: true } }, hidden)).toBe(false)
      expect(isEntryRouteBlocked({ name: 'Login' }, hidden)).toBe(true)
    })

    it('OAuth 往返（离开页面再回来、标记已消失）后仍能找回入口', () => {
      landOn(HIDDEN_PATH, 1)
      expect(captureLoginEntryPath()).toBe(HIDDEN_PATH)

      // 第三方回跳：新的一次页面加载，URL 变了、标记没了，但 sessionStorage 还在。
      landOn('/auth/callback')
      expect(captureLoginEntryPath()).toBe(HIDDEN_PATH)
      expect(resolveLoginPath(hidden)).toBe(HIDDEN_PATH)
    })

    it('sessionStorage 为空的新标签页拿不到入口', () => {
      landOn(HIDDEN_PATH, 1)
      expect(captureLoginEntryPath()).toBe(HIDDEN_PATH)

      sessionStorage.clear() // 另一个标签页 = 另一份 sessionStorage
      landOn('/home')
      expect(captureLoginEntryPath()).toBeNull()
    })
  })

  it('注入的 __APP_CONFIG__ 在不传 settings 时被采用', () => {
    window.__APP_CONFIG__ = settings({ login_entry_public: false, default_home_path: '/home' })
    landOn('/home')
    expect(isLoginEntryHidden()).toBe(true)
    expect(resolveDefaultHomePath()).toBe('/home')
    expect(resolveLoginPath()).toBeNull()
  })
})
