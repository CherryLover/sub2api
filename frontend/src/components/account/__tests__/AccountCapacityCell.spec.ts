import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getRecentRequests } = vi.hoisted(() => ({
  getRecentRequests: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getRecentRequests,
    },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key),
    }),
  }
})

import AccountCapacityCell from '../AccountCapacityCell.vue'

const makeAccount = (id: number) => ({
  id,
  name: `acc-${id}`,
  platform: 'openai',
  type: 'apikey',
  concurrency: 3,
  current_concurrency: 1,
}) as any

const makeSummary = (accountId: number) => ({
  account_id: accountId,
  window_minutes: 15,
  current_concurrency: 1,
  max_concurrency: 3,
  waiting_count: 0,
  total_requests: 12,
  items: [],
  by_api_key: [
    { api_key_id: 4, name: 'tom', count: 2, cost: 0.0001 },
    { api_key_id: 3, name: 'jerry', count: 10, cost: 0.0007 },
  ],
  by_model: [{ model: 'grok-3-mini', count: 12, cost: 0.0008 }],
})

const mountCell = (account: any) => mount(AccountCapacityCell, {
  props: { account },
  global: { stubs: { Teleport: true } },
})

const concurrencyButton = (wrapper: ReturnType<typeof mountCell>) => wrapper.get('[data-testid="capacity-concurrency"]')

describe('AccountCapacityCell concurrency hover summary', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getRecentRequests.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('fetches a 15-minute / limit=1 summary only after hovering for 300ms and shows the busiest key', async () => {
    const account = makeAccount(101)
    getRecentRequests.mockResolvedValue(makeSummary(account.id))
    const wrapper = mountCell(account)

    await concurrencyButton(wrapper).trigger('mouseenter')
    vi.advanceTimersByTime(299)
    expect(getRecentRequests).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1)
    await flushPromises()

    expect(getRecentRequests).toHaveBeenCalledTimes(1)
    expect(getRecentRequests).toHaveBeenCalledWith(account.id, { minutes: 15, limit: 1 })

    const summary = wrapper.get('[data-testid="capacity-load-summary"]')
    expect(summary.text()).toContain('admin.accounts.capacity.load.summary')
    expect(summary.text()).toContain('"current":1')
    expect(summary.text()).toContain('"max":3')
    expect(summary.text()).toContain('"minutes":15')
    expect(summary.text()).toContain('"count":12')
    // 最活跃密钥按 count 取最大，而不是数组第一个
    expect(summary.text()).toContain('"key":"jerry"')
  })

  it('cancels the pending fetch when the pointer leaves before the delay', async () => {
    const account = makeAccount(102)
    getRecentRequests.mockResolvedValue(makeSummary(account.id))
    const wrapper = mountCell(account)

    await concurrencyButton(wrapper).trigger('mouseenter')
    vi.advanceTimersByTime(100)
    await concurrencyButton(wrapper).trigger('mouseleave')
    vi.advanceTimersByTime(500)
    await flushPromises()

    expect(getRecentRequests).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="capacity-load-summary"]').exists()).toBe(false)
  })

  it('hides the summary on mouseleave and reuses the cached result within 30 seconds', async () => {
    const account = makeAccount(103)
    getRecentRequests.mockResolvedValue(makeSummary(account.id))
    const wrapper = mountCell(account)

    await concurrencyButton(wrapper).trigger('mouseenter')
    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(getRecentRequests).toHaveBeenCalledTimes(1)

    await concurrencyButton(wrapper).trigger('mouseleave')
    await flushPromises()
    expect(wrapper.find('[data-testid="capacity-load-summary"]').exists()).toBe(false)

    // 10 秒后再悬停：命中缓存，不再请求
    vi.advanceTimersByTime(10_000)
    await concurrencyButton(wrapper).trigger('mouseenter')
    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(getRecentRequests).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="capacity-load-summary"]').text()).toContain('"count":12')

    // 超过 30 秒缓存过期：重新请求
    await concurrencyButton(wrapper).trigger('mouseleave')
    vi.advanceTimersByTime(31_000)
    await concurrencyButton(wrapper).trigger('mouseenter')
    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(getRecentRequests).toHaveBeenCalledTimes(2)
  })

  it('shows a failure hint instead of throwing when the summary request fails', async () => {
    const account = makeAccount(104)
    getRecentRequests.mockRejectedValue(new Error('boom'))
    const wrapper = mountCell(account)

    await concurrencyButton(wrapper).trigger('mouseenter')
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(wrapper.get('[data-testid="capacity-load-summary"]').text()).toContain('admin.accounts.capacity.load.summaryFailed')
  })

  it('emits openLoad with the account when the concurrency badge is clicked', async () => {
    const account = makeAccount(105)
    const wrapper = mountCell(account)

    await concurrencyButton(wrapper).trigger('click')

    expect(wrapper.emitted('openLoad')).toEqual([[account]])
    expect(getRecentRequests).not.toHaveBeenCalled()
  })
})
