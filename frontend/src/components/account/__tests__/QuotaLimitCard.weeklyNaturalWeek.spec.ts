import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import { mount } from '@vue/test-utils'

// 批次 6 / A3：新建 / 编辑账号弹窗里的周限额默认按自然周重置
// （固定时间 / 周一 / 00:00 / 服务器时区）。QuotaLimitCard + QuotaDimensionRow 是两个弹窗
// 共用的表单区块，这里直接驱动它们，等价于覆盖弹窗的默认值行为。

const { publicSettings } = vi.hoisted(() => ({
  publicSettings: { value: { server_timezone: 'Asia/Shanghai' } as Record<string, unknown> | null }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    get cachedPublicSettings() {
      return publicSettings.value
    }
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${JSON.stringify(params)}` : key
    })
  }
})

import QuotaLimitCard from '../QuotaLimitCard.vue'

type QuotaFormState = {
  totalLimit: number | null
  dailyLimit: number | null
  weeklyLimit: number | null
  dailyResetMode: 'rolling' | 'fixed' | null
  dailyResetHour: number | null
  weeklyResetMode: 'rolling' | 'fixed' | null
  weeklyResetDay: number | null
  weeklyResetHour: number | null
  resetTimezone: string | null
}

function emptyState(): QuotaFormState {
  return {
    totalLimit: null,
    dailyLimit: null,
    weeklyLimit: null,
    dailyResetMode: null,
    dailyResetHour: null,
    weeklyResetMode: null,
    weeklyResetDay: null,
    weeklyResetHour: null,
    resetTimezone: null
  }
}

// 宿主组件把 update:* 事件写回自己的状态，模拟 CreateAccountModal / EditAccountModal
// 里 `:weeklyResetMode="editWeeklyResetMode" @update:weeklyResetMode="editWeeklyResetMode = $event"`
// 那一串绑定，这样子组件的「下一步」判断能读到已更新的 props。
function mountCard(initial: Partial<QuotaFormState> = {}) {
  const state = reactive<QuotaFormState>({ ...emptyState(), ...initial })
  const Host = defineComponent({
    components: { QuotaLimitCard },
    setup() {
      return { state }
    },
    template: `
      <QuotaLimitCard
        :total-limit="state.totalLimit"
        :daily-limit="state.dailyLimit"
        :weekly-limit="state.weeklyLimit"
        :daily-reset-mode="state.dailyResetMode"
        :daily-reset-hour="state.dailyResetHour"
        :weekly-reset-mode="state.weeklyResetMode"
        :weekly-reset-day="state.weeklyResetDay"
        :weekly-reset-hour="state.weeklyResetHour"
        :reset-timezone="state.resetTimezone"
        @update:total-limit="state.totalLimit = $event"
        @update:daily-limit="state.dailyLimit = $event"
        @update:weekly-limit="state.weeklyLimit = $event"
        @update:daily-reset-mode="state.dailyResetMode = $event"
        @update:daily-reset-hour="state.dailyResetHour = $event"
        @update:weekly-reset-mode="state.weeklyResetMode = $event"
        @update:weekly-reset-day="state.weeklyResetDay = $event"
        @update:weekly-reset-hour="state.weeklyResetHour = $event"
        @update:reset-timezone="state.resetTimezone = $event"
      />
    `
  })
  const wrapper = mount(Host)
  return { wrapper, state }
}

async function enableQuotaCard(wrapper: ReturnType<typeof mountCard>['wrapper']) {
  await wrapper.get('[data-testid="quota-limit-toggle"]').trigger('click')
}

function selectValue(wrapper: ReturnType<typeof mountCard>['wrapper'], testId: string) {
  return (wrapper.get(`[data-testid="${testId}"]`).element as HTMLSelectElement).value
}

function selectOptions(wrapper: ReturnType<typeof mountCard>['wrapper'], testId: string) {
  return wrapper
    .get(`[data-testid="${testId}"]`)
    .findAll('option')
    .map((option) => (option.element as HTMLOptionElement).value)
}

describe('QuotaLimitCard weekly natural-week default', () => {
  beforeEach(() => {
    publicSettings.value = { server_timezone: 'Asia/Shanghai' }
  })

  it('previews fixed / Monday / 00:00 / server timezone before any weekly limit is entered, without touching form state', async () => {
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    expect(selectValue(wrapper, 'quota-weekly-reset-mode')).toBe('fixed')
    expect(selectValue(wrapper, 'quota-weekly-reset-day')).toBe('1')
    expect(selectValue(wrapper, 'quota-weekly-reset-hour')).toBe('0')
    expect(selectValue(wrapper, 'quota-weekly-reset-timezone')).toBe('Asia/Shanghai')
    expect(wrapper.text()).toContain('"timezone":"Asia/Shanghai"')

    // 只是预览：没有周限额就不该往表单里写任何重置配置
    expect(state.weeklyResetMode).toBeNull()
    expect(state.weeklyResetDay).toBeNull()
    expect(state.weeklyResetHour).toBeNull()
    expect(state.resetTimezone).toBeNull()
  })

  it('applies the natural-week default to form state once a weekly limit is entered', async () => {
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('50')

    expect(state.weeklyLimit).toBe(50)
    expect(state.weeklyResetMode).toBe('fixed')
    expect(state.weeklyResetDay).toBe(1)
    expect(state.weeklyResetHour).toBe(0)
    expect(state.resetTimezone).toBe('Asia/Shanghai')
    expect(selectValue(wrapper, 'quota-weekly-reset-mode')).toBe('fixed')
  })

  it('falls back to UTC when public settings do not carry server_timezone', async () => {
    publicSettings.value = {}
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    expect(selectValue(wrapper, 'quota-weekly-reset-timezone')).toBe('UTC')
    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('50')
    expect(state.resetTimezone).toBe('UTC')
  })

  it('offers a server timezone that is not in the common list so the select never shows blank', async () => {
    publicSettings.value = { server_timezone: 'Asia/Ho_Chi_Minh' }
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    expect(selectOptions(wrapper, 'quota-weekly-reset-timezone')[0]).toBe('Asia/Ho_Chi_Minh')
    expect(selectValue(wrapper, 'quota-weekly-reset-timezone')).toBe('Asia/Ho_Chi_Minh')
    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('10')
    expect(state.resetTimezone).toBe('Asia/Ho_Chi_Minh')
  })

  it('keeps showing rolling for a legacy account that already has a weekly limit but no reset mode', async () => {
    const { wrapper, state } = mountCard({ weeklyLimit: 100 })

    expect(selectValue(wrapper, 'quota-weekly-reset-mode')).toBe('rolling')
    expect(wrapper.find('[data-testid="quota-weekly-reset-day"]').exists()).toBe(false)

    // 只是改限额数值，不能偷偷把老账号切成固定重置
    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('200')
    expect(state.weeklyLimit).toBe(200)
    expect(state.weeklyResetMode).toBeNull()
    expect(state.resetTimezone).toBeNull()
  })

  it('re-applies the default when a legacy account clears and re-enters its weekly limit', async () => {
    // 留一个总限额：清空唯一的限额会让整张卡片自动关掉（既有行为），这里只验周限额那一行
    const { wrapper, state } = mountCard({ weeklyLimit: 100, totalLimit: 500 })

    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('')
    expect(state.weeklyLimit).toBeNull()
    expect(state.weeklyResetMode).toBeNull()

    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('80')
    expect(state.weeklyResetMode).toBe('fixed')
    expect(state.weeklyResetDay).toBe(1)
    expect(state.weeklyResetHour).toBe(0)
    expect(state.resetTimezone).toBe('Asia/Shanghai')
  })

  it('respects an explicitly chosen rolling window when the limit is entered afterwards', async () => {
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    await wrapper.get('[data-testid="quota-weekly-reset-mode"]').setValue('rolling')
    expect(state.weeklyResetMode).toBe('rolling')

    await wrapper.get('[data-testid="quota-weekly-limit"]').setValue('50')
    expect(state.weeklyResetMode).toBe('rolling')
    expect(state.resetTimezone).toBeNull()
    expect(selectValue(wrapper, 'quota-weekly-reset-mode')).toBe('rolling')
  })

  it('leaves the daily limit on its rolling default', async () => {
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    expect(selectValue(wrapper, 'quota-daily-reset-mode')).toBe('rolling')
    await wrapper.get('[data-testid="quota-daily-limit"]').setValue('10')
    expect(state.dailyLimit).toBe(10)
    expect(state.dailyResetMode).toBeNull()
    expect(state.resetTimezone).toBeNull()
  })

  it('uses the server timezone when any dimension is switched to fixed without a timezone', async () => {
    const { wrapper, state } = mountCard()
    await enableQuotaCard(wrapper)

    await wrapper.get('[data-testid="quota-daily-reset-mode"]').setValue('fixed')
    expect(state.dailyResetMode).toBe('fixed')
    expect(state.dailyResetHour).toBe(0)
    expect(state.resetTimezone).toBe('Asia/Shanghai')
  })
})
