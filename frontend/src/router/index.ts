/**
 * Vue Router configuration for Sub2API frontend
 * Defines all application routes with lazy loading and navigation guards
 */

import {
  createRouter,
  createWebHistory,
  type RouteLocationNormalized,
  type RouteRecordRaw,
} from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useNavigationLoadingState } from '@/composables/useNavigationLoading'
import { useRoutePrefetch } from '@/composables/useRoutePrefetch'
import { getSetupStatus } from '@/api/setup'
import { resolveCompletedSetupRedirectPath } from './setupRedirect'
import { resolveRouteDocumentTitle } from './title'
import {
  captureLoginEntryPath,
  isEntryRoute,
  isEntryRouteBlocked,
  isLoginEntryHidden,
  resolveDefaultHomePath,
  resolveUnauthenticatedTarget,
} from './loginEntry'

// 必须在路由发生第一次导航之前读取：隐藏登录路径的唯一来源就是本次页面加载的 URL。
const initialLoginEntryPath = captureLoginEntryPath()

/**
 * Route definitions with lazy loading
 */
const routes: RouteRecordRaw[] = [
  // ==================== Setup Routes ====================
  {
    path: '/setup',
    name: 'Setup',
    component: () => import('@/views/setup/SetupWizardView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Setup'
    }
  },

  // ==================== Public Routes ====================
  {
    path: '/home',
    name: 'Home',
    component: () => import('@/views/HomeView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Home'
    }
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Login',
      titleKey: 'home.login'
    }
  },
  {
    path: '/key-usage',
    name: 'KeyUsage',
    component: () => import('@/views/KeyUsageView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Key Usage',
    }
  },
  {
    path: '/model-plaza',
    name: 'ModelPlaza',
    component: () => import('@/views/ModelPlazaView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Model Plaza',
      titleKey: 'modelPlaza.title'
    }
  },

  // ==================== User Routes ====================
  {
    // 默认首页可配置（后端 web.default_home_path，默认 /key-usage）。
    // 用函数形式而不是常量，是为了在设置到位之后（注入或异步拉取）每次都重新解析。
    path: '/',
    redirect: () => {
      try {
        return resolveDefaultHomePath(useAppStore().cachedPublicSettings)
      } catch {
        // pinia 尚未激活（部分单测直接 import 路由表）时回落到注入的配置。
        return resolveDefaultHomePath()
      }
    },
  },
  {
    path: '/dashboard',
    name: 'Dashboard',
    component: () => import('@/views/user/DashboardView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'Dashboard',
      titleKey: 'dashboard.title',
      descriptionKey: 'dashboard.welcomeMessage'
    }
  },
  {
    path: '/keys',
    name: 'Keys',
    component: () => import('@/views/user/KeysView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'API Keys',
      titleKey: 'keys.title',
      descriptionKey: 'keys.description'
    }
  },
  {
    path: '/usage',
    name: 'Usage',
    component: () => import('@/views/user/UsageView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'Usage Records',
      titleKey: 'usage.title',
      descriptionKey: 'usage.description'
    }
  },
  {
    path: '/available-channels',
    name: 'UserAvailableChannels',
    component: () => import('@/views/user/AvailableChannelsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'Available Channels',
      titleKey: 'availableChannels.title',
      descriptionKey: 'availableChannels.description'
    }
  },
  {
    path: '/profile',
    name: 'Profile',
    component: () => import('@/views/user/ProfileView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'Profile',
      titleKey: 'profile.title',
      descriptionKey: 'profile.description'
    }
  },
  {
    path: '/subscriptions',
    name: 'Subscriptions',
    component: () => import('@/views/user/SubscriptionsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'My Subscriptions',
      titleKey: 'userSubscriptions.title',
      descriptionKey: 'userSubscriptions.description'
    }
  },

  // ==================== Admin Routes ====================
  {
    path: '/admin',
    redirect: '/admin/dashboard'
  },
  {
    path: '/admin/dashboard',
    name: 'AdminDashboard',
    component: () => import('@/views/admin/DashboardView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Admin Dashboard',
      titleKey: 'admin.dashboard.title',
      descriptionKey: 'admin.dashboard.description'
    }
  },
  {
    path: '/admin/ops',
    name: 'AdminOps',
    component: () => import('@/views/admin/ops/OpsDashboard.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Ops Monitoring',
      titleKey: 'admin.ops.title',
      descriptionKey: 'admin.ops.description'
    }
  },
  {
    path: '/admin/audit-logs',
    name: 'AdminAuditLogs',
    component: () => import('@/views/admin/AuditLogView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Audit Logs',
      titleKey: 'admin.audit.title',
      descriptionKey: 'admin.audit.description'
    }
  },
  {
    path: '/admin/users',
    name: 'AdminUsers',
    component: () => import('@/views/admin/UsersView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'User Management',
      titleKey: 'admin.users.title',
      descriptionKey: 'admin.users.description'
    }
  },
  {
    path: '/admin/groups',
    name: 'AdminGroups',
    component: () => import('@/views/admin/GroupsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Group Management',
      titleKey: 'admin.groups.title',
      descriptionKey: 'admin.groups.description'
    }
  },
  {
    path: '/admin/channels',
    redirect: '/admin/channels/pricing'
  },
  {
    path: '/admin/channels/pricing',
    name: 'AdminChannels',
    component: () => import('@/views/admin/ChannelsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Channel Management',
      titleKey: 'admin.channels.title',
      descriptionKey: 'admin.channels.description'
    }
  },
  {
    path: '/admin/channels/monitor',
    name: 'AdminChannelMonitor',
    component: () => import('@/views/admin/ChannelMonitorView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Channel Monitor',
      titleKey: 'admin.channelMonitor.title',
      descriptionKey: 'admin.channelMonitor.description'
    }
  },
  {
    path: '/monitor',
    name: 'ChannelStatus',
    component: () => import('@/views/user/ChannelStatusView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: false,
      title: 'Channel Status',
      titleKey: 'nav.channelStatus'
    }
  },
  {
    path: '/admin/subscriptions',
    name: 'AdminSubscriptions',
    component: () => import('@/views/admin/SubscriptionsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Subscription Management',
      titleKey: 'admin.subscriptions.title',
      descriptionKey: 'admin.subscriptions.description'
    }
  },
  {
    path: '/admin/accounts',
    name: 'AdminAccounts',
    component: () => import('@/views/admin/AccountsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Account Management',
      titleKey: 'admin.accounts.title',
      descriptionKey: 'admin.accounts.description'
    }
  },
  {
    path: '/admin/announcements',
    name: 'AdminAnnouncements',
    component: () => import('@/views/admin/AnnouncementsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Announcements',
      titleKey: 'admin.announcements.title',
      descriptionKey: 'admin.announcements.description'
    }
  },
  {
    path: '/admin/proxies',
    name: 'AdminProxies',
    component: () => import('@/views/admin/ProxiesView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Proxy Management',
      titleKey: 'admin.proxies.title',
      descriptionKey: 'admin.proxies.description'
    }
  },
  {
    path: '/admin/settings',
    name: 'AdminSettings',
    component: () => import('@/views/admin/SettingsView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'System Settings',
      titleKey: 'admin.settings.title',
      descriptionKey: 'admin.settings.description'
    }
  },
  {
    path: '/admin/prompt-audit',
    name: 'AdminPromptAudit',
    component: () => import('@/features/prompt-audit/PromptAuditView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Prompt Audit',
      titleKey: 'admin.promptAudit.title',
      descriptionKey: 'admin.promptAudit.description',
      requiresRiskControl: true
    }
  },
  {
    path: '/admin/usage',
    name: 'AdminUsage',
    component: () => import('@/views/admin/UsageView.vue'),
    meta: {
      requiresAuth: true,
      requiresAdmin: true,
      title: 'Usage Records',
      titleKey: 'admin.usage.title',
      descriptionKey: 'admin.usage.description'
    }
  },

  // ==================== 404 Not Found ====================
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/views/NotFoundView.vue'),
    meta: {
      title: '404 Not Found'
    }
  }
]

/**
 * Create router instance
 */
const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior(_to, _from, savedPosition) {
    // Scroll to saved position when using browser back/forward
    if (savedPosition) {
      return savedPosition
    }
    // Scroll to top for new routes
    return { top: 0 }
  }
})

/**
 * 隐藏模式下的登录入口路由。
 *
 * 路径来自 `initialLoginEntryPath` —— 即"后端确认本次加载命中了自定义登录路径"时
 * 的当前 URL。它不在上面的静态路由表里，因此不会被编译进 JS 产物；不知道路径的
 * 访问者拿到的 bundle 里根本没有这条路由。
 *
 * 静态路径的匹配优先级高于末尾的 `/:pathMatch(.*)*`，所以后注册也能正常命中。
 */
if (initialLoginEntryPath && isLoginEntryHidden()) {
  router.addRoute({
    path: initialLoginEntryPath,
    name: 'LoginEntry',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: {
      requiresAuth: false,
      title: 'Login',
      titleKey: 'home.login',
      loginEntry: true,
    },
  })
}

/**
 * Navigation guard: Authentication check
 */
let authInitialized = false

// 初始化导航加载状态和预加载
const navigationLoading = useNavigationLoadingState()
// 延迟初始化预加载，传入 router 实例
let routePrefetch: ReturnType<typeof useRoutePrefetch> | null = null
// '/login' 只在登录入口公开时才是一条真实路由；隐藏模式下的登录入口靠
// isLoginRoute() 按路由名放行，路径本身不进这份常量（它会被编译进产物）。
const BACKEND_MODE_ALLOWED_PATHS = ['/login', '/key-usage', '/setup']

function isBackendModePublicRouteAllowed(
  path: string,
  isLoginEntryRoute = false
): boolean {
  // 隐藏登录入口在 backend mode 下和 /login 等价放行。
  if (isLoginEntryRoute) {
    return true
  }

  return BACKEND_MODE_ALLOWED_PATHS.some(
    (allowedPath) => path === allowedPath || path.startsWith(allowedPath)
  )
}

/** 当前导航目标是不是登录页（公开的 /login 或隐藏模式下动态注册的入口）。 */
function isLoginRoute(to: RouteLocationNormalized): boolean {
  return to.name === 'Login' || to.meta.loginEntry === true
}

router.beforeEach(async (to, _from, next) => {
  // 开始导航加载状态
  navigationLoading.startNavigation()

  const authStore = useAuthStore()

  // Restore auth state from localStorage on first navigation (page refresh)
  if (!authInitialized) {
    authStore.checkAuth()
    authInitialized = true
  }

  // Set page title
  const appStore = useAppStore()
  document.title = resolveRouteDocumentTitle(to)

  /**
   * 未登录（或 backend mode 下无权限）被拦下时的落点。
   *
   * 隐藏模式下 resolveUnauthenticatedTarget 会给出默认首页，但 backend mode 只放行
   * 一小撮公开路径——如果默认首页恰好不在其中（例如 /home），把人送过去会立刻被同一条
   * 规则再拦一次，形成重定向死循环。这里兜一层：落点必须是 backend mode 也认的路径，
   * 否则退回始终放行的 /key-usage。
   */
  const blockedTarget = (redirectFullPath?: string) => {
    const target = resolveUnauthenticatedTarget(appStore.cachedPublicSettings, redirectFullPath)
    if (appStore.backendModeEnabled && !isBackendModePublicRouteAllowed(target.path)) {
      return { path: '/key-usage' }
    }
    return target
  }

  // Check if route requires authentication
  const requiresAuth = to.meta.requiresAuth !== false // Default to true
  const requiresAdmin = to.meta.requiresAdmin === true

  if (to.path === '/setup') {
    try {
      const status = await getSetupStatus()
      if (!status.needs_setup) {
        next(resolveCompletedSetupRedirectPath(authStore.isAuthenticated, authStore.isAdmin))
        return
      }
    } catch {
      // If setup status cannot be determined, keep the setup page reachable.
    }
  }

  // If route doesn't require auth, allow access
  if (!requiresAuth) {
    // 登录入口隐藏时：静态的 /login 不再可用，注册/找回密码也只在本标签页已经走过
    // 隐藏入口时才可达。设置尚未加载完（静态部署首次导航）先补齐，避免因为"还不知道
    // 是不是隐藏模式"而漏放一次 /login。
    if (isEntryRoute(to)) {
      if (!appStore.publicSettingsLoaded) {
        try {
          await appStore.fetchPublicSettings()
        } catch (error) {
          console.warn('Failed to load public settings in route guard', error)
        }
      }
      const entrySettings = appStore.cachedPublicSettings
      if (isEntryRouteBlocked(to, entrySettings)) {
        next({ path: resolveDefaultHomePath(entrySettings), replace: true })
        return
      }
    }

    // If already authenticated and trying to access login, redirect to appropriate dashboard
    if (authStore.isAuthenticated && isLoginRoute(to)) {
      // In backend mode, non-admin users should NOT be redirected away from login
      // (they are blocked from all protected routes, so redirecting would cause a loop)
      if (appStore.backendModeEnabled && !authStore.isAdmin) {
        next()
        return
      }
      // Admin users go to admin dashboard, regular users go to user dashboard
      next(authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
      return
    }
    // Model Plaza:公开路由但受「启用开关 + 可选强制登录」双重控制(后端同口径 fail-closed)
    if (to.path === '/model-plaza') {
      if (!appStore.publicSettingsLoaded) {
        try {
          await appStore.fetchPublicSettings()
        } catch (error) {
          console.warn('Failed to load public settings in route guard', error)
        }
      }
      const plazaSettings = appStore.cachedPublicSettings
      // 仅在设置成功加载且明确为 false 时拦截(瞬时加载失败视为未知,由后端 404 兜底)
      if (appStore.publicSettingsLoaded && plazaSettings?.model_plaza_enabled === false) {
        next(
          authStore.isAuthenticated
            ? authStore.isAdmin
              ? '/admin/dashboard'
              : '/dashboard'
            : '/home'
        )
        return
      }
      if (plazaSettings?.model_plaza_require_auth === true && !authStore.isAuthenticated) {
        next(blockedTarget(to.fullPath))
        return
      }
      // Backend mode:登录的非管理员也不可见(匿名由下方公共拦截处理,广场不在白名单)
      if (appStore.backendModeEnabled && authStore.isAuthenticated && !authStore.isAdmin) {
        next(blockedTarget())
        return
      }
    }
    // Backend mode: block public pages for unauthenticated users (except login, key-usage, setup)
    if (appStore.backendModeEnabled && !authStore.isAuthenticated) {
      const isAllowed = isBackendModePublicRouteAllowed(to.path, to.meta.loginEntry === true)
      if (!isAllowed) {
        next(blockedTarget())
        return
      }
    }
    next()
    return
  }

  // Route requires authentication
  if (!authStore.isAuthenticated) {
    // 未登录：公开模式回 /login 并带上 redirect；隐藏模式下只有本标签页已经知道
    // 入口时才回登录页，否则送去默认首页——绝不能因为随手访问一个受保护 URL
    // 就把自定义登录路径透出去。
    next(blockedTarget(to.fullPath))
    return
  }

  // Check admin requirement
  if (requiresAdmin && !authStore.isAdmin) {
    // User is authenticated but not admin, redirect to user dashboard
    next('/dashboard')
    return
  }


  // 公共设置可能尚未加载（App.vue 的 onMounted 异步拉取晚于首次导航，且纯静态部署
  // 无 __APP_CONFIG__ 注入）。此时 cachedPublicSettings 为空会把 risk_control
  // 误判为“未启用”而错误拦截，故这里先确保设置加载完成。
  if (to.meta.requiresRiskControl && !appStore.publicSettingsLoaded) {
    try {
      await appStore.fetchPublicSettings()
    } catch (error) {
      console.warn('Failed to load public settings in route guard', error)
    }
  }

  // Only an explicit value from successfully loaded settings can disable a route.
  // A transient settings failure is unknown state, not a confirmed feature toggle.
  if (
    to.meta.requiresRiskControl &&
    appStore.publicSettingsLoaded &&
    appStore.cachedPublicSettings?.risk_control_enabled === false
  ) {
    next(authStore.isAdmin ? '/admin/settings' : '/dashboard')
    return
  }

  // 简易模式下限制访问某些页面
  if (authStore.isSimpleMode) {
    const restrictedPaths = [
      '/admin/groups',
      '/admin/subscriptions',
      '/subscriptions'
    ]

    if (restrictedPaths.some((path) => to.path.startsWith(path))) {
      // 简易模式下访问受限页面,重定向到仪表板
      next(authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
      return
    }
  }

  // Backend mode: admin gets full access, non-admin blocked
  if (appStore.backendModeEnabled) {
    if (authStore.isAuthenticated && authStore.isAdmin) {
      next()
      return
    }
    const isAllowed = isBackendModePublicRouteAllowed(to.path, to.meta.loginEntry === true)
    if (!isAllowed) {
      next(blockedTarget())
      return
    }
  }

  // All checks passed, allow navigation
  next()
})

/**
 * Navigation guard: End loading and trigger prefetch
 */
router.afterEach((to) => {
  // 结束导航加载状态
  navigationLoading.endNavigation()

  // 懒初始化预加载（首次导航时创建，传入 router 实例）
  if (!routePrefetch) {
    routePrefetch = useRoutePrefetch(router)
  }
  // 触发路由预加载（在浏览器空闲时执行）
  routePrefetch.triggerPrefetch(to)
})

/**
 * Navigation guard: Error handling
 * Handles dynamic import failures caused by deployment updates
 */
router.onError((error) => {
  console.error('Router error:', error)

  // Check if this is a dynamic import failure (chunk loading error)
  const isChunkLoadError =
    error.message?.includes('Failed to fetch dynamically imported module') ||
    error.message?.includes('Loading chunk') ||
    error.message?.includes('Loading CSS chunk') ||
    error.name === 'ChunkLoadError'

  if (isChunkLoadError) {
    // Avoid infinite reload loop by checking sessionStorage
    const reloadKey = 'chunk_reload_attempted'
    const lastReload = sessionStorage.getItem(reloadKey)
    const now = Date.now()

    // Allow reload if never attempted or more than 10 seconds ago
    if (!lastReload || now - parseInt(lastReload) > 10000) {
      sessionStorage.setItem(reloadKey, now.toString())
      console.warn('Chunk load error detected, reloading page to fetch latest version...')
      window.location.reload()
    } else {
      console.error('Chunk load error persists after reload. Please clear browser cache.')
    }
  }
})

export default router
