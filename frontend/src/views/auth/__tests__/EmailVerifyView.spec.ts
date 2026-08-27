import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmailVerifyView from '@/views/auth/EmailVerifyView.vue'

const {
  pushMock,
  showSuccessMock,
  showErrorMock,
  registerMock,
  setTokenMock,
  setPendingAuthSessionMock,
  clearPendingAuthSessionMock,
  getPublicSettingsMock,
  sendVerifyCodeMock,
  sendPendingOAuthVerifyCodeMock,
  persistOAuthTokenContextMock,
  apiClientPostMock,
  authStoreState,
  createTurnstileResetMock,
  verifyActionMock,
} = vi.hoisted(() => ({
  pushMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  registerMock: vi.fn(),
  setTokenMock: vi.fn(),
  setPendingAuthSessionMock: vi.fn(),
  clearPendingAuthSessionMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  sendVerifyCodeMock: vi.fn(),
  sendPendingOAuthVerifyCodeMock: vi.fn(),
  persistOAuthTokenContextMock: vi.fn(),
  apiClientPostMock: vi.fn(),
  createTurnstileResetMock: vi.fn(),
  verifyActionMock: vi.fn(),
  authStoreState: {
    pendingAuthSession: null as null | {
      token: string
      token_field: 'pending_auth_token' | 'pending_oauth_token'
      provider: string
      redirect?: string
      adoption_required?: boolean
      suggested_display_name?: string
      suggested_avatar_url?: string
    }
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    },
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === 'auth.accountCreatedSuccess') {
        return `Account created for ${params?.siteName ?? 'Sub2API'}`
      }
      if (key === 'auth.emailDomainRegistrationLimit') {
        return '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
      }
      return key
    },
    locale: { value: 'en' },
  }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    pendingAuthSession: authStoreState.pendingAuthSession,
    register: (...args: any[]) => registerMock(...args),
    setToken: (...args: any[]) => setTokenMock(...args),
    setPendingAuthSession: (...args: any[]) => setPendingAuthSessionMock(...args),
    clearPendingAuthSession: (...args: any[]) => clearPendingAuthSessionMock(...args),
  }),
  useAppStore: () => ({
    showSuccess: (...args: any[]) => showSuccessMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
    sendVerifyCode: (...args: any[]) => sendVerifyCodeMock(...args),
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCodeMock(...args),
    persistOAuthTokenContext: (...args: any[]) => persistOAuthTokenContextMock(...args),
  }
})

vi.mock('@/api/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiClientPostMock(...args),
  },
}))

describe('EmailVerifyView', () => {
  beforeEach(() => {
    pushMock.mockReset()
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    registerMock.mockReset()
    setTokenMock.mockReset()
    setPendingAuthSessionMock.mockReset()
    clearPendingAuthSessionMock.mockReset()
    getPublicSettingsMock.mockReset()
    sendVerifyCodeMock.mockReset()
    sendPendingOAuthVerifyCodeMock.mockReset()
    persistOAuthTokenContextMock.mockReset()
    apiClientPostMock.mockReset()
    createTurnstileResetMock.mockReset()
    verifyActionMock.mockReset()
    authStoreState.pendingAuthSession = null
    sessionStorage.clear()
    localStorage.clear()

    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    sendPendingOAuthVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    setTokenMock.mockResolvedValue({})
  })

  it('acquires a fresh Tencent proof for each resend action', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockResolvedValue({ countdown: 0 })
    verifyActionMock
      .mockResolvedValueOnce({ token: 'ticket-1', randstr: '@rand-1' })
      .mockResolvedValueOnce({ token: 'ticket-2', randstr: '@rand-2' })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        tencent_captcha_ticket: 'initial-ticket',
        tencent_captcha_randstr: '@initial-rand',
      })
    )

    const CaptchaChallengeStub = defineComponent({
      setup(_, { expose }) {
        expose({ verifyAction: verifyActionMock, reset: createTurnstileResetMock })
        return () => h('div')
      },
    })
    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: CaptchaChallengeStub,
          transition: false,
        },
      },
    })

    await flushPromises()
    const resendButton = () => wrapper.findAll('button').find((button) =>
      button.text().includes('auth.clickToResend')
    )!

    await resendButton().trigger('click')
    await flushPromises()
    await resendButton().trigger('click')
    await flushPromises()

    expect(verifyActionMock).toHaveBeenCalledTimes(2)
    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(2, expect.objectContaining({
      tencent_captcha_ticket: 'ticket-1',
      tencent_captcha_randstr: '@rand-1',
    }))
    expect(sendVerifyCodeMock).toHaveBeenNthCalledWith(3, expect.objectContaining({
      tencent_captcha_ticket: 'ticket-2',
      tencent_captcha_randstr: '@rand-2',
    }))
  })

  it('requires a fresh captcha proof after the initial send-code request fails', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sendVerifyCodeMock.mockRejectedValue(new Error('send failed'))
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'fresh@example.com',
        password: 'secret-123',
        turnstile_token: 'initial-proof',
      })
    )

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: {
            template: '<button data-testid="resend-captcha" @click="$emit(\'verify\', \'fresh-proof\')">verify</button>',
          },
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith(expect.objectContaining({
      turnstile_token: 'initial-proof',
    }))
    expect(wrapper.find('[data-testid="resend-captcha"]').exists()).toBe(true)
  })

  it('sends a verification code for a non-whitelist email domain', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true,
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'first@custom.example',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'first@custom.example' })
    )
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('shows the localized domain quota message when sending a verification code is rejected', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true,
    })
    sendVerifyCodeMock.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message',
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'second@custom.example',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(showErrorMock).toHaveBeenLastCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  it('shows the localized domain quota message when verified registration is rejected', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
      registration_email_domain_quota_enabled: true,
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'second@custom.example',
        password: 'secret-123',
      })
    )
    registerMock.mockRejectedValueOnce({
      reason: 'EMAIL_DOMAIN_REGISTRATION_LIMIT',
      message: 'raw backend message',
    })

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenLastCalledWith(
      '该邮箱域名无法注册新账户。请使用主流邮箱注册；如需使用企业邮箱，请联系客服添加域名白名单。'
    )
  })

  // 域名限量注册开关默认关闭：恢复 PR5423 之前的客户端白名单预检，非白名单域名不发送验证码。
  it('blocks sending a verification code for a non-whitelist email domain when the quota switch is disabled', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: ['allowed.com'],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'first@custom.example',
        password: 'secret-123',
      })
    )

    mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()

    expect(sendVerifyCodeMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('auth.emailSuffixNotAllowedWithAllowed')
  })

  it('keeps the normal email registration flow unchanged', async () => {
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'normal@example.com',
        password: 'secret-456',
      })
    )
    registerMock.mockResolvedValue({})

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          transition: false,
        },
      },
    })

    await flushPromises()
    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith({
      email: 'normal@example.com',
      password: 'secret-456',
      verify_code: '654321',
      turnstile_token: undefined,
      tencent_captcha_ticket: undefined,
      tencent_captcha_randstr: undefined,
    })
    expect(apiClientPostMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('does not require another Tencent proof for final email registration', async () => {
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id',
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: [],
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'normal@example.com',
        password: 'secret-456',
        tencent_captcha_ticket: 'send-code-ticket',
        tencent_captcha_randstr: '@send-code-rand',
      })
    )
    registerMock.mockResolvedValue({})

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: {
            template: '<span />',
            methods: {
              reset: createTurnstileResetMock,
            },
          },
          transition: false,
        },
      },
    })

    await flushPromises()
    expect(sendVerifyCodeMock).toHaveBeenCalledWith(expect.objectContaining({
      tencent_captcha_ticket: 'send-code-ticket',
      tencent_captcha_randstr: '@send-code-rand',
    }))
    expect(JSON.parse(sessionStorage.getItem('register_data') || '{}')).toEqual({
      email: 'normal@example.com',
      password: 'secret-456',
    })

    await wrapper.get('#code').setValue('654321')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(registerMock).toHaveBeenCalledWith(expect.objectContaining({
      email: 'normal@example.com',
      verify_code: '654321',
      tencent_captcha_ticket: undefined,
      tencent_captcha_randstr: undefined,
    }))
  })
})
