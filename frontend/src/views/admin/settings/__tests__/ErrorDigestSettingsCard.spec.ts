import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import ErrorDigestSettingsCard from '../ErrorDigestSettingsCard.vue'

const {
  getErrorDigestConfig,
  updateErrorDigestConfig,
  testErrorDigest,
  showError,
  showSuccess,
  showWarning,
} = vi.hoisted(() => ({
  getErrorDigestConfig: vi.fn(),
  updateErrorDigestConfig: vi.fn(),
  testErrorDigest: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    notifications: {
      getErrorDigestConfig,
      updateErrorDigestConfig,
      testErrorDigest,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning,
  }),
}))

// t() 原样返回键名；带参数时把参数值拼在后面，方便断言插值
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

const configuredConfig = () => ({
  enabled: true,
  schedule: '0 9,21 * * *',
  skip_when_empty: false,
  top_keys: 5,
  updated_at: '2026-09-15T10:00:00Z',
})

const freshConfig = () => ({
  enabled: false,
  schedule: '30 11,17 * * *',
  skip_when_empty: true,
  top_keys: 8,
  updated_at: '',
})

const digestBody = [
  '区间 17:30 – 11:30（18 小时）',
  '失败 128 次：真故障 28，业务拦截 100',
  '',
  '张三 / dev-key：42 次',
  '  上游过载(529) 30、上游错误(504) 12',
  '另有 3 个密钥共 12 次',
].join('\n')

const pushedResult = () => ({
  pushed: true,
  title: '[Sub2API] 报错汇总 11:30',
  body: digestBody,
  summary: {
    start: '2026-09-15T09:30:00Z',
    end: '2026-09-16T03:30:00Z',
    total: 128,
    sla: 28,
    limited: 100,
    groups: [],
    hidden_groups: 3,
    hidden_total: 12,
  },
})

async function mountCard() {
  const wrapper = mount(ErrorDigestSettingsCard, {
    global: {
      stubs: {
        Toggle: ToggleStub,
        Icon: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

const field = (wrapper: Awaited<ReturnType<typeof mountCard>>, id: string) =>
  wrapper.find(`[data-testid="${id}"]`)

describe('ErrorDigestSettingsCard', () => {
  beforeEach(() => {
    getErrorDigestConfig.mockReset()
    updateErrorDigestConfig.mockReset()
    testErrorDigest.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
  })

  it('loads the config on mount and fills every field', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())

    const wrapper = await mountCard()

    expect(getErrorDigestConfig).toHaveBeenCalledTimes(1)
    expect((field(wrapper, 'error-digest-enabled').element as HTMLInputElement).checked).toBe(true)
    expect((field(wrapper, 'error-digest-schedule').element as HTMLInputElement).value).toBe(
      '0 9,21 * * *',
    )
    expect((field(wrapper, 'error-digest-top-keys').element as HTMLInputElement).value).toBe('5')
    expect(
      (field(wrapper, 'error-digest-skip-when-empty').element as HTMLInputElement).checked,
    ).toBe(false)
    expect(field(wrapper, 'error-digest-last-saved').exists()).toBe(true)
    expect(field(wrapper, 'error-digest-result').exists()).toBe(false)
  })

  it('shows backend defaults and no saved timestamp before the first save', async () => {
    getErrorDigestConfig.mockResolvedValue(freshConfig())

    const wrapper = await mountCard()

    expect((field(wrapper, 'error-digest-enabled').element as HTMLInputElement).checked).toBe(false)
    expect((field(wrapper, 'error-digest-schedule').element as HTMLInputElement).value).toBe(
      '30 11,17 * * *',
    )
    expect((field(wrapper, 'error-digest-top-keys').element as HTMLInputElement).value).toBe('8')
    expect(
      (field(wrapper, 'error-digest-skip-when-empty').element as HTMLInputElement).checked,
    ).toBe(true)
    expect(field(wrapper, 'error-digest-last-saved').exists()).toBe(false)
  })

  it('saves the trimmed payload and refreshes from the response', async () => {
    getErrorDigestConfig.mockResolvedValue(freshConfig())
    updateErrorDigestConfig.mockResolvedValue({
      ...freshConfig(),
      enabled: true,
      schedule: '0 9 * * *',
      skip_when_empty: false,
      top_keys: 3,
      updated_at: '2026-09-15T12:00:00Z',
    })

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-enabled').setValue(true)
    await field(wrapper, 'error-digest-schedule').setValue('  0 9 * * *  ')
    await field(wrapper, 'error-digest-skip-when-empty').setValue(false)
    await field(wrapper, 'error-digest-top-keys').setValue('3')
    await field(wrapper, 'error-digest-save').trigger('click')
    await flushPromises()

    expect(updateErrorDigestConfig).toHaveBeenCalledTimes(1)
    expect(updateErrorDigestConfig).toHaveBeenCalledWith({
      enabled: true,
      schedule: '0 9 * * *',
      skip_when_empty: false,
      top_keys: 3,
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.errorDigest.saved')
    expect(showError).not.toHaveBeenCalled()
    expect((field(wrapper, 'error-digest-schedule').element as HTMLInputElement).value).toBe(
      '0 9 * * *',
    )
    expect(field(wrapper, 'error-digest-last-saved').exists()).toBe(true)
  })

  it('falls back to the default key count when the number input is cleared', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    updateErrorDigestConfig.mockResolvedValue(configuredConfig())

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-top-keys').setValue('')
    await field(wrapper, 'error-digest-save').trigger('click')
    await flushPromises()

    expect(updateErrorDigestConfig).toHaveBeenCalledWith(expect.objectContaining({ top_keys: 8 }))
  })

  it('clamps an out-of-range key count before sending it', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    updateErrorDigestConfig.mockResolvedValue({ ...configuredConfig(), top_keys: 50 })

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-top-keys').setValue('999')
    await field(wrapper, 'error-digest-save').trigger('click')
    await flushPromises()

    expect(updateErrorDigestConfig).toHaveBeenCalledWith(expect.objectContaining({ top_keys: 50 }))
  })

  it('surfaces the backend message when saving fails', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    updateErrorDigestConfig.mockRejectedValue({
      status: 400,
      message: 'schedule must be a 5-field cron expression',
    })

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-save').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('schedule must be a 5-field cron expression')
    expect(showSuccess).not.toHaveBeenCalled()
    expect((field(wrapper, 'error-digest-save').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('sends a test digest without a body and previews what the phone would receive', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    testErrorDigest.mockResolvedValue(pushedResult())

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-test').trigger('click')
    await flushPromises()

    // 试推固定按最近 24 小时算，与表单上未保存的值无关，所以不带请求体
    expect(testErrorDigest).toHaveBeenCalledTimes(1)
    expect(testErrorDigest).toHaveBeenCalledWith()
    expect(updateErrorDigestConfig).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.errorDigest.pushed')

    const result = field(wrapper, 'error-digest-result')
    expect(result.exists()).toBe(true)
    expect(result.attributes('data-tone')).toBe('success')
    expect(result.text()).toContain('admin.settings.notifications.errorDigest.resultPushed')
    expect(field(wrapper, 'error-digest-preview').text()).toContain(
      '失败 128 次：真故障 28，业务拦截 100',
    )
    expect(field(wrapper, 'error-digest-preview').text()).toContain('另有 3 个密钥共 12 次')
  })

  it('warns instead of celebrating when the digest was generated but not pushed', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    testErrorDigest.mockResolvedValue({
      ...pushedResult(),
      pushed: false,
      reason: 'bark_disabled',
    })

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-test').trigger('click')
    await flushPromises()

    expect(showWarning).toHaveBeenCalledWith('admin.settings.notifications.errorDigest.notPushed')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()

    const result = field(wrapper, 'error-digest-result')
    expect(result.attributes('data-tone')).toBe('warning')
    expect(result.text()).toContain('admin.settings.notifications.errorDigest.resultNotPushed')
    // 没推出去也把正文贴出来，站长至少能先看看内容对不对
    expect(field(wrapper, 'error-digest-preview').exists()).toBe(true)
  })

  it('shows the backend failure message when the test call is rejected, and re-enables the buttons', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    let reject!: (reason: unknown) => void
    testErrorDigest.mockImplementation(
      () =>
        new Promise((_, rej) => {
          reject = rej
        }),
    )

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-test').trigger('click')
    await flushPromises()

    expect((field(wrapper, 'error-digest-test').element as HTMLButtonElement).disabled).toBe(true)
    expect((field(wrapper, 'error-digest-save').element as HTMLButtonElement).disabled).toBe(true)

    reject({ status: 502, message: 'dial tcp: i/o timeout' })
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('dial tcp: i/o timeout')
    const result = field(wrapper, 'error-digest-result')
    expect(result.attributes('data-tone')).toBe('error')
    expect(result.text()).toContain('dial tcp: i/o timeout')
    expect(field(wrapper, 'error-digest-preview').exists()).toBe(false)
    expect((field(wrapper, 'error-digest-test').element as HTMLButtonElement).disabled).toBe(false)
    expect((field(wrapper, 'error-digest-save').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('disables both buttons and reports the failure when the config cannot be loaded', async () => {
    getErrorDigestConfig.mockRejectedValue({ status: 500, message: 'db is down' })

    const wrapper = await mountCard()

    expect(showError).toHaveBeenCalledWith('db is down')
    expect((field(wrapper, 'error-digest-save').element as HTMLButtonElement).disabled).toBe(true)
    expect((field(wrapper, 'error-digest-test').element as HTMLButtonElement).disabled).toBe(true)

    await field(wrapper, 'error-digest-save').trigger('click')
    await flushPromises()
    expect(updateErrorDigestConfig).not.toHaveBeenCalled()
  })

  it('saves the card instead of submitting the page when Enter is pressed in an input', async () => {
    getErrorDigestConfig.mockResolvedValue(configuredConfig())
    updateErrorDigestConfig.mockResolvedValue(configuredConfig())

    const wrapper = await mountCard()
    await field(wrapper, 'error-digest-schedule').trigger('keydown.enter')
    await flushPromises()

    expect(updateErrorDigestConfig).toHaveBeenCalledTimes(1)
  })
})
