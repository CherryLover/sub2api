import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import BarkNotifySettingsCard from '../BarkNotifySettingsCard.vue'

const { getBarkConfig, updateBarkConfig, testBark, showError, showSuccess, showInfo, showWarning } =
  vi.hoisted(() => ({
    getBarkConfig: vi.fn(),
    updateBarkConfig: vi.fn(),
    testBark: vi.fn(),
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn(),
  }))

vi.mock('@/api', () => ({
  adminAPI: {
    notifications: {
      getBarkConfig,
      updateBarkConfig,
      testBark,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning,
    showInfo,
  }),
}))

// t() 原样返回键名；带参数时把参数值拼在后面，方便断言「延迟 87 ms」这类插值
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${Object.values(params).join(',')}` : key,
  }),
}))

const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () =>
      h('input', {
        ...attrs,
        type: 'checkbox',
        checked: props.modelValue,
        onChange: (event: Event) => {
          emit('update:modelValue', (event.target as HTMLInputElement).checked)
        },
      })
  },
})

const SelectStub = defineComponent({
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: '' },
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue'],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () =>
      h(
        'select',
        {
          ...attrs,
          value: props.modelValue ?? '',
          onChange: (event: Event) => {
            emit('update:modelValue', (event.target as HTMLSelectElement).value)
          },
        },
        (props.options as Array<{ value: string; label: string }>).map((option) =>
          h('option', { key: option.value, value: option.value }, option.label),
        ),
      )
  },
})

const configuredConfig = () => ({
  enabled: true,
  server_url: 'https://bark.example.com',
  device_key: '',
  has_device_key: true,
  group: 'ops',
  level: 'critical',
  sound: 'alarm',
  click_url: 'https://sub2api.example.com/admin/ops',
  notify_on_resolve: false,
  updated_at: '2026-09-04T10:00:00Z',
})

const freshConfig = () => ({
  enabled: false,
  server_url: 'https://api.day.app',
  device_key: '',
  has_device_key: false,
  group: 'sub2api',
  level: 'active',
  sound: '',
  click_url: '',
  notify_on_resolve: true,
  updated_at: '',
})

const okTestResult = () => ({
  ok: true,
  ping_ok: true,
  status_code: 200,
  message: 'success',
  latency_ms: 87,
})

async function mountCard() {
  const wrapper = mount(BarkNotifySettingsCard, {
    global: {
      stubs: {
        Toggle: ToggleStub,
        Select: SelectStub,
        Icon: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

const field = (wrapper: Awaited<ReturnType<typeof mountCard>>, id: string) =>
  wrapper.find(`[data-testid="${id}"]`)

describe('BarkNotifySettingsCard', () => {
  beforeEach(() => {
    getBarkConfig.mockReset()
    updateBarkConfig.mockReset()
    testBark.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
  })

  it('loads the config on mount and fills every field, never echoing the device key', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())

    const wrapper = await mountCard()

    expect(getBarkConfig).toHaveBeenCalledTimes(1)
    expect((field(wrapper, 'bark-enabled').element as HTMLInputElement).checked).toBe(true)
    expect((field(wrapper, 'bark-server-url').element as HTMLInputElement).value).toBe(
      'https://bark.example.com',
    )
    expect((field(wrapper, 'bark-group').element as HTMLInputElement).value).toBe('ops')
    expect((field(wrapper, 'bark-level').element as HTMLSelectElement).value).toBe('critical')
    expect((field(wrapper, 'bark-sound').element as HTMLInputElement).value).toBe('alarm')
    expect((field(wrapper, 'bark-click-url').element as HTMLInputElement).value).toBe(
      'https://sub2api.example.com/admin/ops',
    )
    expect((field(wrapper, 'bark-notify-on-resolve').element as HTMLInputElement).checked).toBe(
      false,
    )

    const deviceKey = field(wrapper, 'bark-device-key').element as HTMLInputElement
    expect(deviceKey.type).toBe('password')
    expect(deviceKey.value).toBe('')
    expect(deviceKey.placeholder).toBe(
      'admin.settings.notifications.bark.deviceKeyConfiguredPlaceholder',
    )
    expect(field(wrapper, 'bark-device-key-configured').exists()).toBe(true)
    expect(field(wrapper, 'bark-device-key-warning').exists()).toBe(false)
    expect(field(wrapper, 'bark-last-saved').exists()).toBe(true)
    expect(field(wrapper, 'bark-level-hint').text()).toBe(
      'admin.settings.notifications.bark.levelHints.critical',
    )
  })

  it('saves the full payload with an empty device_key when the input is left blank, then refreshes from the response', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    updateBarkConfig.mockResolvedValue({
      ...configuredConfig(),
      group: 'sub2api-prod',
      level: 'passive',
      updated_at: '2026-09-04T11:00:00Z',
    })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-group').setValue('  sub2api-prod  ')
    await field(wrapper, 'bark-level').setValue('passive')
    await field(wrapper, 'bark-save').trigger('click')
    await flushPromises()

    expect(updateBarkConfig).toHaveBeenCalledTimes(1)
    expect(updateBarkConfig).toHaveBeenCalledWith({
      enabled: true,
      server_url: 'https://bark.example.com',
      device_key: '',
      group: 'sub2api-prod',
      level: 'passive',
      sound: 'alarm',
      click_url: 'https://sub2api.example.com/admin/ops',
      notify_on_resolve: false,
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.bark.saved')
    expect(showError).not.toHaveBeenCalled()
    expect((field(wrapper, 'bark-group').element as HTMLInputElement).value).toBe('sub2api-prod')
    expect((field(wrapper, 'bark-level').element as HTMLSelectElement).value).toBe('passive')
    expect(field(wrapper, 'bark-level-hint').text()).toBe(
      'admin.settings.notifications.bark.levelHints.passive',
    )
    expect(field(wrapper, 'bark-device-key-configured').exists()).toBe(true)
  })

  it('sends a newly typed device_key, then clears the input and flips the configured marker', async () => {
    getBarkConfig.mockResolvedValue(freshConfig())
    updateBarkConfig.mockResolvedValue({
      ...freshConfig(),
      enabled: true,
      has_device_key: true,
      updated_at: '2026-09-04T12:00:00Z',
    })

    const wrapper = await mountCard()
    expect(field(wrapper, 'bark-device-key-configured').exists()).toBe(false)
    expect(field(wrapper, 'bark-last-saved').exists()).toBe(false)

    await field(wrapper, 'bark-device-key').setValue('abc123')
    await field(wrapper, 'bark-enabled').setValue(true)
    await field(wrapper, 'bark-save').trigger('click')
    await flushPromises()

    expect(updateBarkConfig).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: true, device_key: 'abc123' }),
    )
    expect((field(wrapper, 'bark-device-key').element as HTMLInputElement).value).toBe('')
    expect(field(wrapper, 'bark-device-key-configured').exists()).toBe(true)
    expect(field(wrapper, 'bark-last-saved').exists()).toBe(true)
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.bark.saved')
  })

  it('blocks saving with an inline warning when push is enabled but no device key exists', async () => {
    getBarkConfig.mockResolvedValue(freshConfig())

    const wrapper = await mountCard()
    expect(field(wrapper, 'bark-device-key-warning').exists()).toBe(false)

    await field(wrapper, 'bark-enabled').setValue(true)
    expect(field(wrapper, 'bark-device-key-warning').exists()).toBe(true)

    await field(wrapper, 'bark-save').trigger('click')
    await flushPromises()

    expect(updateBarkConfig).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.settings.notifications.bark.deviceKeyRequired')
    expect(showSuccess).not.toHaveBeenCalled()

    // 填上 Key 后提示消失，保存放行
    updateBarkConfig.mockResolvedValue({ ...freshConfig(), enabled: true, has_device_key: true })
    await field(wrapper, 'bark-device-key').setValue('k')
    expect(field(wrapper, 'bark-device-key-warning').exists()).toBe(false)
    await field(wrapper, 'bark-save').trigger('click')
    await flushPromises()
    expect(updateBarkConfig).toHaveBeenCalledTimes(1)
  })

  it('surfaces the backend message when saving fails', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    updateBarkConfig.mockRejectedValue({ status: 400, message: 'server_url must be https' })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-save').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('server_url must be https')
    expect(showSuccess).not.toHaveBeenCalled()
    expect((field(wrapper, 'bark-save').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('sends a test notification with the current unsaved form values plus title/body', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    testBark.mockResolvedValue(okTestResult())

    const wrapper = await mountCard()
    await field(wrapper, 'bark-server-url').setValue('https://custom.example')
    await field(wrapper, 'bark-send-test').trigger('click')
    await flushPromises()

    expect(updateBarkConfig).not.toHaveBeenCalled()
    expect(testBark).toHaveBeenCalledTimes(1)
    expect(testBark).toHaveBeenCalledWith({
      enabled: true,
      server_url: 'https://custom.example',
      device_key: '',
      group: 'ops',
      level: 'critical',
      sound: 'alarm',
      click_url: 'https://sub2api.example.com/admin/ops',
      notify_on_resolve: false,
      title: 'admin.settings.notifications.bark.testTitle',
      body: 'admin.settings.notifications.bark.testBody',
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.bark.sent')

    const result = field(wrapper, 'bark-test-result')
    expect(result.exists()).toBe(true)
    expect(result.text()).toContain('admin.settings.notifications.bark.resultSent')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultLatency:87')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultStatus:200')
  })

  it('test connection reports the ping result and latency without a title/body', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    testBark.mockResolvedValue({ ...okTestResult(), latency_ms: 42 })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-test-connection').trigger('click')
    await flushPromises()

    expect(testBark).toHaveBeenCalledTimes(1)
    const payload = testBark.mock.calls[0][0]
    expect(payload).not.toHaveProperty('title')
    expect(payload).not.toHaveProperty('body')
    expect(payload).toMatchObject({ server_url: 'https://bark.example.com', device_key: '' })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.bark.connectionOk:42')
    expect(showError).not.toHaveBeenCalled()

    const result = field(wrapper, 'bark-test-result')
    expect(result.attributes('data-tone')).toBe('success')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultPingOk')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultLatency:42')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultStatus:200')
  })

  it('test connection with ping ok but no device key shows an info headline instead of a failure', async () => {
    getBarkConfig.mockResolvedValue(freshConfig())
    testBark.mockResolvedValue({
      ok: false,
      ping_ok: true,
      status_code: 0,
      message: '未配置设备 Key，仅测试了服务器连通性',
      latency_ms: 42,
    })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-test-connection').trigger('click')
    await flushPromises()

    // 没有设备 Key 也放行「测试连接」，由后端只做探活
    expect(testBark).toHaveBeenCalledTimes(1)
    expect(testBark.mock.calls[0][0]).toMatchObject({ device_key: '' })
    expect(showInfo).toHaveBeenCalledWith(
      'admin.settings.notifications.bark.connectionOkNoDeviceKey:42',
    )
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()

    const result = field(wrapper, 'bark-test-result')
    expect(result.attributes('data-tone')).toBe('info')
    expect(result.text()).toContain('admin.settings.notifications.bark.connectionOkNoDeviceKey:42')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultLatency:42')
    expect(result.text()).not.toContain('admin.settings.notifications.bark.resultStatus')
  })

  it('test connection with ping failed but push ok shows a warning, not a failure', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    testBark.mockResolvedValue({
      ok: true,
      ping_ok: false,
      status_code: 200,
      message: 'success',
      latency_ms: 120,
    })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-test-connection').trigger('click')
    await flushPromises()

    expect(showWarning).toHaveBeenCalledWith(
      'admin.settings.notifications.bark.connectionPingFailedPushOk',
    )
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()

    const result = field(wrapper, 'bark-test-result')
    expect(result.attributes('data-tone')).toBe('warning')
    expect(result.text()).toContain('admin.settings.notifications.bark.connectionPingFailedPushOk')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultStatus:200')
  })

  it('test connection with both ping and push failed shows the failure with the backend message', async () => {
    getBarkConfig.mockResolvedValue(freshConfig())
    testBark.mockResolvedValue({
      ok: false,
      ping_ok: false,
      status_code: 0,
      message: 'connection refused',
      latency_ms: 0,
    })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-test-connection').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      'admin.settings.notifications.bark.connectionFailedWithMessage:connection refused',
    )
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showInfo).not.toHaveBeenCalled()
    expect(showWarning).not.toHaveBeenCalled()

    const result = field(wrapper, 'bark-test-result')
    expect(result.attributes('data-tone')).toBe('error')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultPingFailed')
    expect(result.text()).toContain('connection refused')
  })

  it('send test only looks at ok, even when the ping failed', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    testBark.mockResolvedValue({
      ok: true,
      ping_ok: false,
      status_code: 200,
      message: 'success',
      latency_ms: 65,
    })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-send-test').trigger('click')
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.bark.sent')
    expect(showWarning).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()

    const result = field(wrapper, 'bark-test-result')
    expect(result.attributes('data-tone')).toBe('success')
    expect(result.text()).toContain('admin.settings.notifications.bark.resultSent')
  })

  it('sends an empty group when the field is cleared and shows the backend default afterwards', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    updateBarkConfig.mockResolvedValue({ ...configuredConfig(), group: 'sub2api' })

    const wrapper = await mountCard()
    await field(wrapper, 'bark-group').setValue('   ')
    await field(wrapper, 'bark-save').trigger('click')
    await flushPromises()

    expect(updateBarkConfig).toHaveBeenCalledWith(expect.objectContaining({ group: '' }))
    expect((field(wrapper, 'bark-group').element as HTMLInputElement).value).toBe('sub2api')
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.bark.saved')
  })

  it('shows the backend failure message when the test call is rejected, and re-enables the buttons', async () => {
    getBarkConfig.mockResolvedValue(configuredConfig())
    let reject!: (reason: unknown) => void
    testBark.mockImplementation(
      () =>
        new Promise((_, rej) => {
          reject = rej
        }),
    )

    const wrapper = await mountCard()
    await field(wrapper, 'bark-send-test').trigger('click')
    await flushPromises()

    expect((field(wrapper, 'bark-send-test').element as HTMLButtonElement).disabled).toBe(true)
    expect((field(wrapper, 'bark-test-connection').element as HTMLButtonElement).disabled).toBe(
      true,
    )
    expect((field(wrapper, 'bark-save').element as HTMLButtonElement).disabled).toBe(true)

    reject({ status: 502, message: 'dial tcp: i/o timeout' })
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('dial tcp: i/o timeout')
    expect(field(wrapper, 'bark-test-result').text()).toContain('dial tcp: i/o timeout')
    expect((field(wrapper, 'bark-send-test').element as HTMLButtonElement).disabled).toBe(false)
    expect((field(wrapper, 'bark-test-connection').element as HTMLButtonElement).disabled).toBe(
      false,
    )
    expect((field(wrapper, 'bark-save').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('refuses to send a test notification when there is no device key at all', async () => {
    getBarkConfig.mockResolvedValue(freshConfig())

    const wrapper = await mountCard()
    await field(wrapper, 'bark-send-test').trigger('click')
    await flushPromises()

    expect(testBark).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(
      'admin.settings.notifications.bark.deviceKeyRequiredForTest',
    )
  })
})
