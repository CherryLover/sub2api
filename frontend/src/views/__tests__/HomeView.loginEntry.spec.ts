import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

import HomeView from '../HomeView.vue'
import { resetLoginEntryCacheForTests } from '@/router/loginEntry'

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email?: string } | null,
    checkAuth: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function mountHome(settings: Record<string, unknown> = {}) {
  appStore.cachedPublicSettings = { ...settings }
  return mount(HomeView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

function linkTargets(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.findAllComponents(RouterLinkStub).map((link) => link.props('to'))
}

describe('HomeView 登录入口', () => {
  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    localStorage.clear()
    sessionStorage.clear()
    delete window.__APP_CONFIG__
    delete window.__LOGIN_ENTRY__
    window.history.replaceState({}, '', '/home')
    resetLoginEntryCacheForTests()
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })

  it.each([undefined, true])('登录入口公开（login_entry_public=%s）时首页展示登录入口', (loginPublic) => {
    const settings = loginPublic === undefined ? {} : { login_entry_public: loginPublic }

    const wrapper = mountHome(settings)
    expect(linkTargets(wrapper)).toContain('/login')
  })

  it('登录入口隐藏时首页完全不渲染登录入口', () => {
    const wrapper = mountHome({ login_entry_public: false })
    const targets = linkTargets(wrapper)
    expect(targets).not.toContain('/login')
    expect(JSON.stringify(targets)).not.toContain('login')
  })

  it('已登录用户在隐藏模式下仍然能看到进入面板的入口', () => {
    authStore.isAuthenticated = true

    const wrapper = mountHome({ login_entry_public: false })
    expect(linkTargets(wrapper)).toContain('/dashboard')
  })

  it('本标签页已经走过隐藏入口时，首页可以再指回那条路径', () => {
    window.history.replaceState({}, '', '/j7q2m9x4vk3p')
    window.__LOGIN_ENTRY__ = 1
    resetLoginEntryCacheForTests()

    const wrapper = mountHome({ login_entry_public: false })
    expect(linkTargets(wrapper)).toContain('/j7q2m9x4vk3p')
  })
})
