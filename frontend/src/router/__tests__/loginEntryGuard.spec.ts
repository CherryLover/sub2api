/**
 * 隐藏登录入口在真实导航守卫上的行为。
 *
 * 用 feature-access.spec.ts 的同款做法：mock 掉 vue-router 的 createRouter，
 * 把 router/index.ts 注册的 beforeEach 抓出来直接跑，这样跑的是真守卫而不是复刻。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const HIDDEN_PATH = '/j7q2m9x4vk3p'

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  addedRoutes: [] as Record<string, any>[],
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: true,
  cachedPublicSettings: null as null | Record<string, unknown>,
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn(() => ({
    beforeEach: vi.fn((guard: NavigationGuard) => {
      routerHarness.guard = guard
    }),
    afterEach: vi.fn(),
    onError: vi.fn(),
    addRoute: vi.fn((route: Record<string, any>) => {
      routerHarness.addedRoutes.push(route)
    }),
  })),
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({}) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

/**
 * 每个用例都重新 import router/index.ts：隐藏入口路径是在模块加载时从当前 URL
 * 捕获的，重新加载才能模拟"这一次页面加载落在哪条 URL 上"。
 */
async function loadRouter(landingPath: string, marker?: number) {
  vi.resetModules()
  routerHarness.guard = null
  routerHarness.addedRoutes = []
  window.history.replaceState({}, '', landingPath)
  if (marker === undefined) {
    delete window.__LOGIN_ENTRY__
  } else {
    window.__LOGIN_ENTRY__ = marker
  }
  await import('@/router')
  if (!routerHarness.guard) throw new Error('router guard was not registered')
  return routerHarness.guard
}

async function navigate(
  guard: NavigationGuard,
  to: { path: string; name?: string; meta?: Record<string, unknown> }
) {
  const next = vi.fn()
  await guard(
    {
      path: to.path,
      fullPath: to.path,
      name: to.name,
      params: {},
      meta: { requiresAuth: false, ...(to.meta ?? {}) },
    },
    {},
    next
  )
  return next
}

const HIDDEN_SETTINGS = { login_entry_public: false, default_home_path: '/key-usage', custom_menu_items: [] }
const PUBLIC_SETTINGS = { login_entry_public: true, default_home_path: '/home', custom_menu_items: [] }

describe('登录入口隐藏时的导航守卫', () => {
  beforeEach(() => {
    sessionStorage.clear()
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.hasPendingAuthSession = false
    appStore.backendModeEnabled = false
    appStore.publicSettingsLoaded = true
    appStore.fetchPublicSettings.mockReset()
    delete window.__APP_CONFIG__
  })

  it('隐藏模式下 /login 被送回默认首页', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    const guard = await loadRouter('/home')

    const next = await navigate(guard, { path: '/login', name: 'Login' })
    expect(next).toHaveBeenCalledWith({ path: '/key-usage', replace: true })
  })

  it('隐藏模式下未登录访问受保护页落到默认首页，不带任何登录路径', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    const guard = await loadRouter('/home')

    const next = await navigate(guard, { path: '/dashboard', name: 'Dashboard', meta: { requiresAuth: true } })
    expect(next).toHaveBeenCalledWith({ path: '/key-usage' })
    expect(JSON.stringify(next.mock.calls)).not.toContain(HIDDEN_PATH)
    expect(JSON.stringify(next.mock.calls)).not.toContain('/login')
  })

  it('隐藏模式下找回密码在没走过入口时不可达', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    const guard = await loadRouter('/home')

    expect(await navigate(guard, { path: '/forgot-password', name: 'ForgotPassword' })).toHaveBeenCalledWith({
      path: '/key-usage',
      replace: true,
    })
  })

  it('隐藏模式不会误伤免登录页 / 邮件入口 / OAuth 回调 / 支付回跳', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    const guard = await loadRouter('/home')

    for (const route of [
      { path: '/key-usage', name: 'KeyUsage' },
      { path: '/reset-password', name: 'ResetPassword' },
      { path: '/auth/callback', name: 'OAuthCallback' },
      { path: '/auth/linuxdo/callback', name: 'LinuxDoOAuthCallback' },
      { path: '/payment/result', name: 'PaymentResult' },
      { path: '/legal/terms', name: 'LegalDocument' },
    ]) {
      const next = await navigate(guard, route)
      expect(next, `${route.path} 应该原样放行`).toHaveBeenCalledWith()
    }
  })

  it('命中自定义路径时注册隐藏登录路由，并放行', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    window.__APP_CONFIG__ = HIDDEN_SETTINGS as never
    const guard = await loadRouter(HIDDEN_PATH, 1)

    expect(routerHarness.addedRoutes).toHaveLength(1)
    expect(routerHarness.addedRoutes[0]?.path).toBe(HIDDEN_PATH)
    expect(routerHarness.addedRoutes[0]?.meta?.loginEntry).toBe(true)

    const next = await navigate(guard, {
      path: HIDDEN_PATH,
      name: 'LoginEntry',
      meta: { loginEntry: true },
    })
    expect(next).toHaveBeenCalledWith()
  })

  it('走过入口后同一标签页的注册页可达', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    window.__APP_CONFIG__ = HIDDEN_SETTINGS as never
    const guard = await loadRouter(HIDDEN_PATH, 1)

    const next = await navigate(guard, { path: '/register', name: 'Register' })
    expect(next).toHaveBeenCalledWith()
  })

  it('已登录用户访问隐藏登录入口会被送去仪表盘（与 /login 同口径）', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    window.__APP_CONFIG__ = HIDDEN_SETTINGS as never
    authStore.isAuthenticated = true
    const guard = await loadRouter(HIDDEN_PATH, 1)

    const next = await navigate(guard, {
      path: HIDDEN_PATH,
      name: 'LoginEntry',
      meta: { loginEntry: true },
    })
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it('backend mode 下隐藏登录入口照常放行', async () => {
    appStore.cachedPublicSettings = HIDDEN_SETTINGS
    window.__APP_CONFIG__ = HIDDEN_SETTINGS as never
    appStore.backendModeEnabled = true
    const guard = await loadRouter(HIDDEN_PATH, 1)

    const next = await navigate(guard, {
      path: HIDDEN_PATH,
      name: 'LoginEntry',
      meta: { loginEntry: true },
    })
    expect(next).toHaveBeenCalledWith()
  })

  it('公开模式一切照旧（回归）', async () => {
    appStore.cachedPublicSettings = PUBLIC_SETTINGS
    const guard = await loadRouter('/home')

    expect(routerHarness.addedRoutes).toHaveLength(0)
    expect(await navigate(guard, { path: '/login', name: 'Login' })).toHaveBeenCalledWith()
    expect(await navigate(guard, { path: '/register', name: 'Register' })).toHaveBeenCalledWith()

    const protectedNext = await navigate(guard, {
      path: '/dashboard',
      name: 'Dashboard',
      meta: { requiresAuth: true },
    })
    expect(protectedNext).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/dashboard' } })
  })

  it('设置尚未加载时会先补齐再判定 /login（静态部署）', async () => {
    appStore.cachedPublicSettings = null
    appStore.publicSettingsLoaded = false
    appStore.fetchPublicSettings.mockImplementation(async () => {
      appStore.cachedPublicSettings = HIDDEN_SETTINGS
      appStore.publicSettingsLoaded = true
      return HIDDEN_SETTINGS
    })
    const guard = await loadRouter('/home')

    const next = await navigate(guard, { path: '/login', name: 'Login' })
    expect(appStore.fetchPublicSettings).toHaveBeenCalled()
    expect(next).toHaveBeenCalledWith({ path: '/key-usage', replace: true })
  })
})

/**
 * backend mode 只放行一小撮公开路径。隐藏模式下未登录访问会被送去默认首页，
 * 如果默认首页不在那份白名单里（例如 /home），会被同一条规则再拦一次 —— 必须
 * 退回始终放行的 /key-usage，否则就是重定向死循环。
 */
describe('backend mode 下的隐藏登录入口', () => {
  beforeEach(() => {
    sessionStorage.clear()
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.hasPendingAuthSession = false
    appStore.backendModeEnabled = true
    appStore.publicSettingsLoaded = true
    appStore.fetchPublicSettings.mockReset()
    delete window.__APP_CONFIG__
  })

  it('默认首页不在 backend mode 白名单里时退回 /key-usage，不打转', async () => {
    appStore.cachedPublicSettings = {
      login_entry_public: false,
      default_home_path: '/home',
      custom_menu_items: [],
    }
    const guard = await loadRouter('/home')

    const next = await navigate(guard, { path: '/dashboard', name: 'Dashboard', meta: { requiresAuth: true } })
    expect(next).toHaveBeenCalledWith({ path: '/key-usage' })
  })

  it('默认首页本身就在白名单里时照常使用', async () => {
    appStore.cachedPublicSettings = {
      login_entry_public: false,
      default_home_path: '/key-usage',
      custom_menu_items: [],
    }
    const guard = await loadRouter('/home')

    const next = await navigate(guard, { path: '/dashboard', name: 'Dashboard', meta: { requiresAuth: true } })
    expect(next).toHaveBeenCalledWith({ path: '/key-usage' })
  })

  it('公开模式下 backend mode 仍然把人送去 /login（回归）', async () => {
    appStore.cachedPublicSettings = { login_entry_public: true, custom_menu_items: [] }
    const guard = await loadRouter('/home')

    const next = await navigate(guard, { path: '/dashboard', name: 'Dashboard', meta: { requiresAuth: true } })
    expect(next).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/dashboard' } })
  })
})
