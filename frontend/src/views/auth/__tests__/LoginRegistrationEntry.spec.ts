import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'

const getPublicSettingsMock = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute: { value: { query: {} } },
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    login: vi.fn(),
    loginWithPasskey: vi.fn()
  }),
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
    isTotp2FARequired: () => false
  }
})

const CaptchaChallengeStub = defineComponent({
  setup(_, { expose }) {
    expose({ verifyAction: vi.fn(), reset: vi.fn() })
    return () => h('div')
  }
})

function mountLogin() {
  return mount(LoginView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        RouterLink: true,
        TurnstileWidget: CaptchaChallengeStub,
        Icon: true,
        LoginAgreementPrompt: true,
        TotpLoginModal: true
      }
    }
  })
}

function baseSettings(overrides: Record<string, unknown> = {}) {
  return {
    turnstile_enabled: false,
    turnstile_site_key: '',
    backend_mode_enabled: false,
    password_reset_enabled: false,
    passkey_enabled: false,
    ...overrides
  }
}

describe('login page registration entry', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
  })

  it('renders the sign-up link when registration is enabled', async () => {
    getPublicSettingsMock.mockResolvedValue(baseSettings({ registration_enabled: true }))
    const wrapper = mountLogin()
    await flushPromises()

    expect(wrapper.find('router-link-stub[to="/register"]').exists()).toBe(true)
  })

  it('hides the sign-up link when registration is disabled', async () => {
    getPublicSettingsMock.mockResolvedValue(baseSettings({ registration_enabled: false }))
    const wrapper = mountLogin()
    await flushPromises()

    expect(wrapper.find('router-link-stub[to="/register"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('auth.dontHaveAccount')
  })

  it('hides the sign-up link when public settings are missing the flag', async () => {
    getPublicSettingsMock.mockResolvedValue(baseSettings())
    const wrapper = mountLogin()
    await flushPromises()

    expect(wrapper.find('router-link-stub[to="/register"]').exists()).toBe(false)
  })

  it('hides the sign-up link when public settings fail to load', async () => {
    getPublicSettingsMock.mockRejectedValue(new Error('network down'))
    const wrapper = mountLogin()
    await flushPromises()

    expect(wrapper.find('router-link-stub[to="/register"]').exists()).toBe(false)
  })
})
