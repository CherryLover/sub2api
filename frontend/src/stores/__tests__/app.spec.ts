import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAppStore } from '@/stores/app'
import { getPublicSettings } from '@/api/auth'
import type { PublicSettings } from '@/types'

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })

  return { promise, resolve, reject }
}

function createPublicSettings(overrides: Partial<PublicSettings> = {}): PublicSettings {
  return {
    registration_enabled: false,
    registration_email_suffix_whitelist: [],
    turnstile_enabled: false,
    turnstile_site_key: '',
    doc_url: '',
    risk_control_enabled: false,
    custom_endpoints: [],
    backend_mode_enabled: false,
    version: '1.0.0',
    channel_monitor_enabled: true,
    available_channels_enabled: false,
    service_quota_enabled: false,
    ...overrides,
  }
}

// Mock API 模块
vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn(),
}))

describe('useAppStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    localStorage.clear()
    vi.mocked(getPublicSettings).mockReset()
    // 清除 window.__APP_CONFIG__
    delete (window as any).__APP_CONFIG__
  })

  afterEach(() => {
    vi.useRealTimers()
    localStorage.clear()
  })

  // --- Toast 消息管理 ---

  describe('Toast 消息管理', () => {
    it('showSuccess 创建 success 类型 toast', () => {
      const store = useAppStore()
      const id = store.showSuccess('操作成功')

      expect(id).toMatch(/^toast-/)
      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('success')
      expect(store.toasts[0].message).toBe('操作成功')
    })

    it('showError 创建 error 类型 toast', () => {
      const store = useAppStore()
      store.showError('出错了')

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('error')
      expect(store.toasts[0].message).toBe('出错了')
    })

    it('showWarning 创建 warning 类型 toast', () => {
      const store = useAppStore()
      store.showWarning('警告信息')

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('warning')
    })

    it('showInfo 创建 info 类型 toast', () => {
      const store = useAppStore()
      store.showInfo('提示信息')

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('info')
    })

    it('toast 在指定 duration 后自动消失', () => {
      const store = useAppStore()
      store.showSuccess('临时消息', 3000)

      expect(store.toasts).toHaveLength(1)

      vi.advanceTimersByTime(3000)

      expect(store.toasts).toHaveLength(0)
    })

    it('hideToast 移除指定 toast', () => {
      const store = useAppStore()
      const id = store.showSuccess('消息1')
      store.showError('消息2')

      expect(store.toasts).toHaveLength(2)

      store.hideToast(id)

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('error')
    })

    it('clearAllToasts 清除所有 toast', () => {
      const store = useAppStore()
      store.showSuccess('消息1')
      store.showError('消息2')
      store.showWarning('消息3')

      expect(store.toasts).toHaveLength(3)

      store.clearAllToasts()

      expect(store.toasts).toHaveLength(0)
    })

    it('hasActiveToasts 正确反映 toast 状态', () => {
      const store = useAppStore()
      expect(store.hasActiveToasts).toBe(false)

      store.showSuccess('消息')
      expect(store.hasActiveToasts).toBe(true)

      store.clearAllToasts()
      expect(store.hasActiveToasts).toBe(false)
    })

    it('多个 toast 的 ID 唯一', () => {
      const store = useAppStore()
      const id1 = store.showSuccess('消息1')
      const id2 = store.showSuccess('消息2')
      const id3 = store.showSuccess('消息3')

      expect(id1).not.toBe(id2)
      expect(id2).not.toBe(id3)
    })
  })

  // --- 侧边栏 ---

  describe('侧边栏管理', () => {
    it('toggleSidebar 切换折叠状态', () => {
      const store = useAppStore()
      expect(store.sidebarCollapsed).toBe(false)

      store.toggleSidebar()
      expect(store.sidebarCollapsed).toBe(true)

      store.toggleSidebar()
      expect(store.sidebarCollapsed).toBe(false)
    })

    it('setSidebarCollapsed 直接设置状态', () => {
      const store = useAppStore()

      store.setSidebarCollapsed(true)
      expect(store.sidebarCollapsed).toBe(true)

      store.setSidebarCollapsed(false)
      expect(store.sidebarCollapsed).toBe(false)
    })

    it('sidebarScrollTop 默认为 0 且可读写', () => {
      const store = useAppStore()
      expect(store.sidebarScrollTop).toBe(0)

      store.sidebarScrollTop = 256
      expect(store.sidebarScrollTop).toBe(256)

      store.sidebarScrollTop = 0
      expect(store.sidebarScrollTop).toBe(0)
    })

    it('toggleMobileSidebar 切换移动端状态', () => {
      const store = useAppStore()
      expect(store.mobileOpen).toBe(false)

      store.toggleMobileSidebar()
      expect(store.mobileOpen).toBe(true)

      store.toggleMobileSidebar()
      expect(store.mobileOpen).toBe(false)
    })
  })

  // --- Loading 状态 ---

  describe('Loading 状态管理', () => {
    it('setLoading 管理引用计数', () => {
      const store = useAppStore()
      expect(store.loading).toBe(false)

      store.setLoading(true)
      expect(store.loading).toBe(true)

      store.setLoading(true) // 两次 true
      expect(store.loading).toBe(true)

      store.setLoading(false) // 第一次 false，计数还是 1
      expect(store.loading).toBe(true)

      store.setLoading(false) // 第二次 false，计数为 0
      expect(store.loading).toBe(false)
    })

    it('setLoading(false) 不会使计数为负', () => {
      const store = useAppStore()

      store.setLoading(false)
      store.setLoading(false)
      expect(store.loading).toBe(false)

      store.setLoading(true)
      expect(store.loading).toBe(true)

      store.setLoading(false)
      expect(store.loading).toBe(false)
    })

    it('withLoading 自动管理 loading 状态', async () => {
      const store = useAppStore()

      const result = await store.withLoading(async () => {
        expect(store.loading).toBe(true)
        return 'done'
      })

      expect(result).toBe('done')
      expect(store.loading).toBe(false)
    })

    it('withLoading 错误时也恢复 loading 状态', async () => {
      const store = useAppStore()

      await expect(
        store.withLoading(async () => {
          throw new Error('操作失败')
        })
      ).rejects.toThrow('操作失败')

      expect(store.loading).toBe(false)
    })

    it('withLoadingAndError 错误时显示 toast 并返回 null', async () => {
      const store = useAppStore()

      const result = await store.withLoadingAndError(async () => {
        throw new Error('网络错误')
      })

      expect(result).toBeNull()
      expect(store.loading).toBe(false)
      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('error')
    })
  })

  // --- reset ---

  describe('reset', () => {
    it('重置所有 UI 状态', () => {
      const store = useAppStore()

      store.setSidebarCollapsed(true)
      store.setLoading(true)
      store.showSuccess('消息')

      store.reset()

      expect(store.sidebarCollapsed).toBe(false)
      expect(store.loading).toBe(false)
      expect(store.toasts).toHaveLength(0)
    })
  })

  // --- 公开设置 ---

  describe('公开设置加载', () => {
    it('并发调用复用并等待同一个请求，包括 force 调用', async () => {
      const deferred = createDeferred<PublicSettings>()
      vi.mocked(getPublicSettings).mockReturnValue(deferred.promise)
      const settings = createPublicSettings({ version: '2.0.0' })
      const store = useAppStore()

      const first = store.fetchPublicSettings()
      const second = store.fetchPublicSettings()
      const forced = store.fetchPublicSettings(true)

      expect(getPublicSettings).toHaveBeenCalledTimes(1)

      const settled = vi.fn()
      void first.then(settled)
      void second.then(settled)
      void forced.then(settled)
      await Promise.resolve()
      expect(settled).not.toHaveBeenCalled()

      deferred.resolve(settings)
      await expect(Promise.all([first, second, forced])).resolves.toEqual([
        settings,
        settings,
        settings,
      ])
      expect(store.publicSettingsLoaded).toBe(true)
      expect(store.cachedPublicSettings?.version).toBe('2.0.0')
    })

    it('force 在无活动请求时绕过缓存，刷新期间的普通调用等待刷新结果', async () => {
      const initial = createPublicSettings({ doc_url: 'https://docs.initial.test' })
      vi.mocked(getPublicSettings).mockResolvedValueOnce(initial)
      const store = useAppStore()
      await store.fetchPublicSettings()

      const deferred = createDeferred<PublicSettings>()
      const updated = createPublicSettings({ doc_url: 'https://docs.updated.test' })
      vi.mocked(getPublicSettings).mockReturnValueOnce(deferred.promise)

      const refresh = store.fetchPublicSettings(true)
      const duringRefresh = store.fetchPublicSettings()

      expect(getPublicSettings).toHaveBeenCalledTimes(2)

      deferred.resolve(updated)
      await expect(Promise.all([refresh, duringRefresh])).resolves.toEqual([updated, updated])
      expect(store.docUrl).toBe('https://docs.updated.test')

      await expect(store.fetchPublicSettings()).resolves.toEqual(updated)
      expect(getPublicSettings).toHaveBeenCalledTimes(2)
    })

    it('并发请求失败时所有调用得到 null，且不会标记设置已加载', async () => {
      const deferred = createDeferred<PublicSettings>()
      vi.mocked(getPublicSettings).mockReturnValue(deferred.promise)
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
      const store = useAppStore()

      const first = store.fetchPublicSettings()
      const second = store.fetchPublicSettings()
      deferred.reject(new Error('network unavailable'))

      await expect(Promise.all([first, second])).resolves.toEqual([null, null])
      expect(getPublicSettings).toHaveBeenCalledTimes(1)
      expect(store.publicSettingsLoaded).toBe(false)
      expect(store.cachedPublicSettings).toBeNull()
      consoleError.mockRestore()
    })

    it('从 window.__APP_CONFIG__ 初始化', () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = {
        version: '1.0.0',
        doc_url: 'https://docs.test.com',
      }

      const store = useAppStore()
      const result = store.initFromInjectedConfig()

      expect(result).toBe(true)
      expect(store.siteVersion).toBe('1.0.0')
      expect(store.docUrl).toBe('https://docs.test.com')
      expect(store.publicSettingsLoaded).toBe(true)
    })

    it('无注入配置时返回 false', () => {
      const store = useAppStore()
      const result = store.initFromInjectedConfig()

      expect(result).toBe(false)
      expect(store.publicSettingsLoaded).toBe(false)
    })

    it('clearPublicSettingsCache 清除缓存', () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = { version: 'test' }
      const store = useAppStore()
      store.initFromInjectedConfig()

      expect(store.publicSettingsLoaded).toBe(true)

      store.clearPublicSettingsCache()

      expect(store.publicSettingsLoaded).toBe(false)
      expect(store.cachedPublicSettings).toBeNull()
    })

    it('注入快照带版本号时不额外请求接口', async () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = createPublicSettings({ version: 'internal-rc-6a1452d' })

      const store = useAppStore()
      await store.fetchPublicSettings()

      expect(getPublicSettings).not.toHaveBeenCalled()
      expect(store.siteVersion).toBe('internal-rc-6a1452d')
    })

    it('注入快照缺版本号时，首次 fetch 就回落读接口补齐', async () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = createPublicSettings({ version: '' })
      vi.mocked(getPublicSettings).mockResolvedValue(
        createPublicSettings({ version: 'internal-rc-6a1452d' }),
      )

      const store = useAppStore()
      await store.fetchPublicSettings()

      expect(getPublicSettings).toHaveBeenCalledTimes(1)
      expect(store.siteVersion).toBe('internal-rc-6a1452d')
    })

    it('注入快照已被 initFromInjectedConfig 套用时，缺版本号仍会回落读接口', async () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = createPublicSettings({ version: '' })
      vi.mocked(getPublicSettings).mockResolvedValue(
        createPublicSettings({ version: 'internal-rc-6a1452d' }),
      )

      const store = useAppStore()
      expect(store.initFromInjectedConfig()).toBe(true)
      expect(store.siteVersion).toBe('')

      await store.fetchPublicSettings()

      expect(getPublicSettings).toHaveBeenCalledTimes(1)
      expect(store.siteVersion).toBe('internal-rc-6a1452d')

      await store.fetchPublicSettings()
      expect(getPublicSettings).toHaveBeenCalledTimes(1)
    })

    it('接口也拿不到版本号时不会反复请求', async () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = createPublicSettings({ version: '' })
      vi.mocked(getPublicSettings).mockResolvedValue(createPublicSettings({ version: '' }))

      const store = useAppStore()
      await store.fetchPublicSettings()
      await store.fetchPublicSettings()
      await store.fetchPublicSettings()

      expect(getPublicSettings).toHaveBeenCalledTimes(1)
      expect(store.siteVersion).toBe('')
    })

    it('fetchPublicSettings(force) 会同步更新运行时注入配置', async () => {
      vi.mocked(getPublicSettings).mockResolvedValue(
        createPublicSettings({ doc_url: 'https://docs.updated.test', version: '2.0.0' })
      )

      const store = useAppStore()
      await store.fetchPublicSettings(true)

      expect((window as any).__APP_CONFIG__.doc_url).toBe('https://docs.updated.test')
      expect((window as any).__APP_CONFIG__.version).toBe('2.0.0')
    })
  })
})
