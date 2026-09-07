import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getRecentRequests, listErrorLogs } = vi.hoisted(() => ({
  getRecentRequests: vi.fn(),
  listErrorLogs: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getRecentRequests,
    },
  },
}))

vi.mock('@/api/admin/ops', () => ({
  listErrorLogs,
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

import AccountLoadDrawer from '../AccountLoadDrawer.vue'

// 抽屉壳子只负责显示/关闭，这里用桩把三个插槽平铺出来便于断言内容
const SideDrawerStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: `
    <div v-if="show" data-test="drawer">
      <div data-test="drawer-title">{{ title }}</div>
      <slot name="subtitle" />
      <slot name="actions" />
      <slot />
    </div>
  `,
}

const account = {
  id: 2,
  name: 'acc-two',
  platform: 'openai',
  type: 'apikey',
  concurrency: 3,
  current_concurrency: 1,
} as any

const recentRequests = {
  account_id: 2,
  window_minutes: 15,
  current_concurrency: 1,
  max_concurrency: 3,
  waiting_count: 0,
  total_requests: 12,
  items: [
    {
      id: 4854,
      request_id: 'req-1',
      created_at: '2026-09-07T09:59:30Z',
      user: { id: 1, email: 'u1@test.com' },
      api_key: { id: 3, name: 'jerry' },
      model: 'grok-3-mini',
      upstream_model: 'grok-4.3',
      request_type: 'chat',
      stream: false,
      duration_ms: 1200,
      first_token_ms: 300,
      input_tokens: 7,
      output_tokens: 111,
      cache_read_tokens: 0,
      total_cost: 0.000072,
      actual_cost: 0.000072,
    },
    {
      id: 4855,
      request_id: 'req-2',
      created_at: '2026-09-07T09:58:00Z',
      user: { id: 1, email: 'u1@test.com' },
      api_key: { id: 4, name: 'tom' },
      model: 'grok-3-mini',
      upstream_model: 'grok-3-mini',
      request_type: 'chat',
      stream: true,
      duration_ms: 800,
      first_token_ms: null,
      input_tokens: 10,
      output_tokens: 20,
      cache_read_tokens: 5,
      total_cost: 0.0001,
      actual_cost: 0.0001,
    },
  ],
  by_api_key: [
    { api_key_id: 3, name: 'jerry', count: 10, cost: 0.0007 },
    { api_key_id: 4, name: 'tom', count: 2, cost: 0.0001 },
  ],
  by_model: [{ model: 'grok-3-mini', count: 12, cost: 0.0008 }],
}

const errorLogsResponse = {
  items: [
    {
      id: 900,
      created_at: '2026-09-07T09:57:00Z',
      phase: 'upstream',
      type: 'rate_limit',
      status_code: 429,
      message: 'upstream rate limited',
    },
  ],
  total: 1,
  page: 1,
  page_size: 20,
  pages: 1,
}

const mountDrawer = (props: { show: boolean; account: any }) => mount(AccountLoadDrawer, {
  props,
  global: {
    stubs: {
      SideDrawer: SideDrawerStub,
      PlatformIcon: true,
      Icon: true,
    },
  },
})

describe('AccountLoadDrawer', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-07T10:00:00.000Z'))
    getRecentRequests.mockReset().mockResolvedValue(recentRequests)
    listErrorLogs.mockReset().mockResolvedValue(errorLogsResponse)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not fetch anything while closed', async () => {
    mountDrawer({ show: false, account })
    await flushPromises()

    expect(getRecentRequests).not.toHaveBeenCalled()
    expect(listErrorLogs).not.toHaveBeenCalled()
  })

  it('loads the 15-minute window on open and queries errors for the same window', async () => {
    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()

    expect(getRecentRequests).toHaveBeenCalledTimes(1)
    expect(getRecentRequests).toHaveBeenCalledWith(2, { minutes: 15, limit: 50 })
    expect(listErrorLogs).toHaveBeenCalledTimes(1)
    expect(listErrorLogs).toHaveBeenCalledWith(expect.objectContaining({
      account_id: 2,
      page: 1,
      page_size: 20,
      start_time: '2026-09-07T09:45:00.000Z',
      end_time: '2026-09-07T10:00:00.000Z',
    }))

    const text = wrapper.text()
    // 头部：账号 / 并发 / 等待 / 总请求 / 延迟提示
    expect(wrapper.get('[data-testid="load-drawer-account"]').text()).toContain('acc-two')
    expect(wrapper.get('[data-testid="load-drawer-concurrency"]').text()).toContain('1')
    expect(wrapper.get('[data-testid="load-drawer-concurrency"]').text()).toContain('3')
    expect(wrapper.get('[data-testid="load-drawer-total"]').text()).toContain('"count":12')
    expect(wrapper.get('[data-testid="load-drawer-delay-hint"]').text()).toContain('admin.accounts.capacity.load.delayHint')
    expect(wrapper.get('[data-testid="load-window-15"]').attributes('aria-pressed')).toBe('true')

    // chips
    const keyChips = wrapper.findAll('[data-testid="load-chip-key"]')
    expect(keyChips).toHaveLength(2)
    expect(keyChips[0].text()).toContain('jerry')
    expect(keyChips[0].text()).toContain('"count":10')
    expect(keyChips[0].text()).toContain('$0.0007')
    const modelChips = wrapper.findAll('[data-testid="load-chip-model"]')
    expect(modelChips).toHaveLength(1)
    expect(modelChips[0].text()).toContain('grok-3-mini')

    // 请求表
    const rows = wrapper.findAll('[data-testid="load-request-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('u1@test.com')
    expect(rows[0].text()).toContain('jerry')
    expect(rows[0].text()).toContain('grok-4.3')
    expect(rows[0].text()).toContain('1.20s')
    expect(rows[0].text()).toContain('300ms')
    expect(rows[0].text()).toContain('$0.000072')
    expect(rows[1].text()).toContain('stream')
    expect(rows[1].text()).toContain('(+5)')
    expect(text).toContain('admin.accounts.capacity.load.requestsLimitHint')

    // 同期错误
    const errorRows = wrapper.findAll('[data-testid="load-error-row"]')
    expect(errorRows).toHaveLength(1)
    expect(errorRows[0].text()).toContain('429')
    expect(errorRows[0].text()).toContain('upstream rate limited')
  })

  it('switches to a 5-minute window and refetches both requests and errors with matching params', async () => {
    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()

    await wrapper.get('[data-testid="load-window-5"]').trigger('click')
    await flushPromises()

    expect(getRecentRequests).toHaveBeenCalledTimes(2)
    expect(getRecentRequests).toHaveBeenLastCalledWith(2, { minutes: 5, limit: 50 })
    expect(listErrorLogs).toHaveBeenLastCalledWith(expect.objectContaining({
      account_id: 2,
      start_time: '2026-09-07T09:55:00.000Z',
      end_time: '2026-09-07T10:00:00.000Z',
    }))
    expect(wrapper.get('[data-testid="load-window-5"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="load-window-15"]').attributes('aria-pressed')).toBe('false')

    // 同一窗口重复点击不再请求
    await wrapper.get('[data-testid="load-window-5"]').trigger('click')
    await flushPromises()
    expect(getRecentRequests).toHaveBeenCalledTimes(2)

    // 60 分钟
    await wrapper.get('[data-testid="load-window-60"]').trigger('click')
    await flushPromises()
    expect(getRecentRequests).toHaveBeenLastCalledWith(2, { minutes: 60, limit: 50 })
    expect(listErrorLogs).toHaveBeenLastCalledWith(expect.objectContaining({
      start_time: '2026-09-07T09:00:00.000Z',
    }))
  })

  it('keeps the request data visible when the error-log request fails', async () => {
    listErrorLogs.mockRejectedValue(new Error('ops down'))

    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()

    expect(wrapper.findAll('[data-testid="load-chip-key"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="load-request-row"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="load-drawer-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="load-drawer-errors-failed"]').text()).toContain('admin.accounts.capacity.load.errorsFailed')
    expect(wrapper.findAll('[data-testid="load-error-row"]')).toHaveLength(0)
  })

  it('shows a retry prompt when the request data fails but still lists errors', async () => {
    getRecentRequests.mockRejectedValue(new Error('404'))

    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()

    expect(wrapper.get('[data-testid="load-drawer-error"]').text()).toContain('admin.accounts.capacity.load.loadFailed')
    expect(wrapper.findAll('[data-testid="load-chip-key"]')).toHaveLength(0)
    expect(wrapper.findAll('[data-testid="load-error-row"]')).toHaveLength(1)

    // 重试：主数据恢复
    getRecentRequests.mockResolvedValue(recentRequests)
    await wrapper.get('[data-testid="load-drawer-error"] button').trigger('click')
    await flushPromises()
    expect(getRecentRequests).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="load-drawer-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="load-chip-key"]')).toHaveLength(2)
  })

  it('refreshes on demand and shows an empty state when the window has no requests', async () => {
    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()

    getRecentRequests.mockResolvedValue({ ...recentRequests, total_requests: 0, items: [], by_api_key: [], by_model: [] })
    listErrorLogs.mockResolvedValue({ ...errorLogsResponse, items: [] })
    await wrapper.get('[data-testid="load-drawer-refresh"]').trigger('click')
    await flushPromises()

    expect(getRecentRequests).toHaveBeenCalledTimes(2)
    expect(listErrorLogs).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="load-drawer-requests-empty"]').text()).toContain('admin.accounts.capacity.load.emptyRequests')
    expect(wrapper.get('[data-testid="load-drawer-errors-empty"]').text()).toContain('admin.accounts.capacity.load.errorsEmpty')
    expect(wrapper.findAll('[data-testid="load-chip-key"]')).toHaveLength(0)
  })

  it('resets to the 15-minute window and reloads when reopened', async () => {
    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()
    await wrapper.get('[data-testid="load-window-60"]').trigger('click')
    await flushPromises()

    await wrapper.setProps({ show: false })
    await flushPromises()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getRecentRequests).toHaveBeenLastCalledWith(2, { minutes: 15, limit: 50 })
    expect(wrapper.get('[data-testid="load-window-15"]').attributes('aria-pressed')).toBe('true')
  })

  it('forwards close from the drawer shell', async () => {
    const wrapper = mountDrawer({ show: true, account })
    await flushPromises()

    wrapper.findComponent(SideDrawerStub).vm.$emit('close')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
