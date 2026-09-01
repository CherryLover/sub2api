import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import TotpSetupModal from '@/components/user/profile/TotpSetupModal.vue'
import TotpDisableDialog from '@/components/user/profile/TotpDisableDialog.vue'

const mocks = vi.hoisted(() => ({
  showSuccess: vi.fn(),
  showError: vi.fn(),
  initiateSetup: vi.fn(),
  enable: vi.fn(),
  disable: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: mocks.showSuccess,
    showError: mocks.showError
  })
}))

vi.mock('@/api', () => ({
  totpAPI: {
    initiateSetup: mocks.initiateSetup,
    enable: mocks.enable,
    disable: mocks.disable
  }
}))

const flushPromises = async () => {
  await Promise.resolve()
  await Promise.resolve()
}

describe('TOTP 弹窗错误提示', () => {
  beforeEach(() => {
    mocks.showSuccess.mockReset()
    mocks.showError.mockReset()
    mocks.initiateSetup.mockReset()
    mocks.enable.mockReset()
    mocks.disable.mockReset()

    mocks.initiateSetup.mockResolvedValue({
      qr_code_url: 'otpauth://totp/Sub2API:test?secret=ABC123',
      secret: 'ABC123',
      setup_token: 'setup-token'
    })
    mocks.enable.mockResolvedValue({ success: true })
    mocks.disable.mockResolvedValue({ success: true })
  })

  it('TotpSetupModal 失败时改用 toast 并不渲染内联错误', async () => {
    mocks.initiateSetup.mockRejectedValue({
      response: { data: { message: 'setup failed' } }
    })

    const wrapper = mount(TotpSetupModal)
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('correct horse battery staple')
    await wrapper.get('button[type="button"].btn-primary').trigger('click')
    await flushPromises()

    expect(mocks.showError).toHaveBeenCalledWith('setup failed')
    expect(wrapper.text()).not.toContain('setup failed')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })

  it('TotpDisableDialog 失败时改用 toast 并不渲染内联错误', async () => {
    mocks.disable.mockRejectedValue({
      response: { data: { message: 'disable failed' } }
    })

    const wrapper = mount(TotpDisableDialog)
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('correct horse battery staple')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(mocks.showError).toHaveBeenCalledWith('disable failed')
    expect(wrapper.text()).not.toContain('disable failed')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })
})
