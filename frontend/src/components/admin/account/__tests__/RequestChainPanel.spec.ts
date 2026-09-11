import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RequestChain } from '@/types'

const { getRequestChain } = vi.hoisted(() => ({ getRequestChain: vi.fn() }))

vi.mock('@/api/admin/ops', () => ({ getRequestChain }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    // 运行时 i18n 构建不支持编译消息，这里把 key + 参数一起吐出来便于断言
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${Object.values(params).join(',')}` : key
    })
  }
})

vi.mock('@/utils/format', async () => {
  const actual = await vi.importActual<typeof import('@/utils/format')>('@/utils/format')
  return {
    ...actual,
    formatTime: (value: string) => `t:${value}`
  }
})

import RequestChainPanel from '../RequestChainPanel.vue'

// 抽屉壳子只负责显示/关闭，用桩把插槽平铺出来便于断言内容
const SideDrawerStub = {
  props: ['show', 'title', 'width'],
  emits: ['close'],
  template: `
    <div v-if="show" data-test="drawer">
      <div data-test="drawer-title">{{ title }}</div>
      <slot name="subtitle" />
      <slot name="actions" />
      <slot />
    </div>
  `
}

const mountPanel = (props: { show: boolean; clientRequestId: string | null }) =>
  mount(RequestChainPanel, {
    props,
    global: {
      stubs: { SideDrawer: SideDrawerStub, Icon: true }
    }
  })

// 真实形状：同一个账号连撞 3 次 502，最后仍由它成功
const recoveredSameAccount: RequestChain = {
  client_request_id: 'req-recovered',
  created_at: '2026-09-10T06:59:39Z',
  model: 'gpt-5.6-sol',
  requested_model: 'gpt-5.6-sol',
  stream: true,
  outcome: 'recovered',
  client_status_code: 200,
  attempts: [
    { seq: 1, at: '2026-09-10T06:59:39Z', account_id: 7, account_name: 'anvizanviz80', platform: 'openai', upstream_status_code: 502, message: '上游过载', kind: 'overloaded' },
    { seq: 2, at: '2026-09-10T06:59:52Z', account_id: 7, account_name: 'anvizanviz80', platform: 'openai', upstream_status_code: 502, message: '上游过载', kind: 'overloaded' },
    { seq: 3, at: '2026-09-10T07:00:04Z', account_id: 7, account_name: 'anvizanviz80', platform: 'openai', upstream_status_code: 502, message: '上游过载', kind: 'overloaded' }
  ],
  final: {
    account_id: 7,
    account_name: 'anvizanviz80',
    succeeded: true,
    time_to_first_token_ms: 18500,
    total_tokens: 1883,
    cost: 0.0212
  }
}

// 从账号 5 切到账号 6 之后成功
const recoveredCrossAccount: RequestChain = {
  client_request_id: 'req-switch',
  created_at: '2026-09-10T07:10:00Z',
  model: 'gpt-5.6-sol',
  requested_model: 'gpt-5.6',
  stream: false,
  outcome: 'recovered',
  client_status_code: 200,
  attempts: [
    { seq: 1, at: '2026-09-10T07:10:00Z', account_id: 5, account_name: 'acc-five', platform: 'openai', upstream_status_code: 429, message: 'rate limited', kind: 'rate_limit' }
  ],
  final: { account_id: 6, account_name: 'acc-six', succeeded: true, time_to_first_token_ms: null, total_tokens: 42, cost: null }
}

// 5/6/7/8 四个账号全试一遍最后失败
const failedChain: RequestChain = {
  client_request_id: 'req-failed',
  created_at: '2026-09-10T07:20:00Z',
  model: 'gpt-5.6-sol',
  requested_model: 'gpt-5.6-sol',
  stream: true,
  outcome: 'failed',
  client_status_code: 502,
  attempts: [
    { seq: 1, at: '2026-09-10T07:20:00Z', account_id: 5, account_name: 'acc-five', platform: 'openai', upstream_status_code: 502, message: 'overloaded', kind: 'overloaded' },
    { seq: 2, at: '2026-09-10T07:20:05Z', account_id: 6, account_name: 'acc-six', platform: 'openai', upstream_status_code: 502, message: 'overloaded', kind: 'overloaded' },
    { seq: 3, at: '2026-09-10T07:20:10Z', account_id: 7, account_name: 'acc-seven', platform: 'openai', upstream_status_code: 502, message: 'overloaded', kind: 'overloaded' },
    { seq: 4, at: '2026-09-10T07:20:15Z', account_id: 8, account_name: 'acc-eight', platform: 'openai', upstream_status_code: 503, message: 'unavailable', kind: 'overloaded' }
  ],
  final: { account_id: 8, account_name: 'acc-eight', succeeded: false, time_to_first_token_ms: null, total_tokens: null, cost: null }
}

describe('RequestChainPanel', () => {
  beforeEach(() => {
    getRequestChain.mockReset().mockResolvedValue(recoveredSameAccount)
  })

  it('does not fetch anything while closed', async () => {
    mountPanel({ show: false, clientRequestId: 'req-recovered' })
    await flushPromises()
    expect(getRequestChain).not.toHaveBeenCalled()
  })

  it('does not fetch when opened without a request id', async () => {
    mountPanel({ show: true, clientRequestId: null })
    await flushPromises()
    expect(getRequestChain).not.toHaveBeenCalled()
  })

  it('renders three attempts on one account and a green success footer', async () => {
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    expect(getRequestChain).toHaveBeenCalledWith('req-recovered')

    // 顶部结论：已自动恢复
    const outcome = wrapper.get('[data-testid="chain-outcome"]')
    expect(outcome.classes()).toContain('bg-emerald-50')
    expect(wrapper.get('[data-testid="chain-outcome-title"]').text()).toBe('admin.accounts.requestChain.outcomeRecovered')
    expect(wrapper.get('[data-testid="chain-outcome-hint"]').text()).toBe('admin.accounts.requestChain.recoveredHint|3,anvizanviz80')

    // 模型 · 时间 · 流式
    const meta = wrapper.get('[data-testid="chain-meta"]').text()
    expect(meta).toContain('gpt-5.6-sol')
    expect(meta).toContain('t:2026-09-10T06:59:39Z')
    expect(meta).toContain('admin.accounts.requestChain.stream')
    expect(wrapper.find('[data-testid="chain-requested-model"]').exists()).toBe(false)

    // 时间线
    const attempts = wrapper.findAll('[data-testid="chain-attempt"]')
    expect(attempts).toHaveLength(3)
    expect(attempts[0].text()).toContain('①')
    expect(attempts[2].text()).toContain('③')
    expect(attempts[0].get('[data-testid="chain-attempt-account"]').text()).toBe('admin.accounts.requestChain.account|7,anvizanviz80')
    expect(attempts[0].get('[data-testid="chain-attempt-message"]').text()).toBe('上游过载')
    // 恢复了的链路：过程中的错误一律用柔和色，不用红色吓人
    attempts.forEach(attempt => {
      expect(attempt.get('[data-testid="chain-attempt-status"]').text()).toBe('502')
      expect(attempt.get('[data-testid="chain-attempt-status"]').classes()).toContain('bg-amber-100')
      expect(attempt.get('[data-testid="chain-attempt-status"]').classes()).not.toContain('bg-red-100')
    })

    // 收尾
    const final = wrapper.get('[data-testid="chain-final"]')
    expect(final.classes()).toContain('bg-emerald-50/70')
    expect(final.text()).toContain('✓')
    expect(final.get('[data-testid="chain-final-account"]').text()).toBe('admin.accounts.requestChain.account|7,anvizanviz80')
    expect(final.get('[data-testid="chain-final-status"]').text()).toBe('admin.accounts.requestChain.finalSuccess')
    const metrics = final.get('[data-testid="chain-final-metrics"]').text()
    expect(metrics).toContain('admin.accounts.requestChain.firstToken|18.5s')
    expect(metrics).toContain('admin.accounts.requestChain.tokens|1,883')
    expect(metrics).toContain('$0.0212')
  })

  it('shows the account switch and the requested model when they differ', async () => {
    getRequestChain.mockResolvedValue(recoveredCrossAccount)
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-switch' })
    await flushPromises()

    expect(wrapper.get('[data-testid="chain-attempt-account"]').text()).toBe('admin.accounts.requestChain.account|5,acc-five')
    expect(wrapper.get('[data-testid="chain-final-account"]').text()).toBe('admin.accounts.requestChain.account|6,acc-six')
    expect(wrapper.get('[data-testid="chain-requested-model"]').text()).toContain('admin.accounts.requestChain.requestedModel|gpt-5.6')
    expect(wrapper.get('[data-testid="chain-meta"]').text()).toContain('admin.accounts.requestChain.nonStream')
    // 缺失的指标不画占位，只留有值的那一项
    expect(wrapper.get('[data-testid="chain-final-metrics"]').text()).toBe('admin.accounts.requestChain.tokens|42')
  })

  it('ends a failed chain in red and reports the status the client received', async () => {
    getRequestChain.mockResolvedValue(failedChain)
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-failed' })
    await flushPromises()

    const outcome = wrapper.get('[data-testid="chain-outcome"]')
    expect(outcome.classes()).toContain('bg-red-50')
    expect(wrapper.get('[data-testid="chain-outcome-title"]').text()).toBe('admin.accounts.requestChain.outcomeFailed')
    expect(wrapper.get('[data-testid="chain-outcome-hint"]').text()).toBe('admin.accounts.requestChain.failedHint|4,502')

    const attempts = wrapper.findAll('[data-testid="chain-attempt"]')
    expect(attempts).toHaveLength(4)
    // 只有压垮这次请求的最后一下是红的，前面几次仍是"换个账号再试"的过程
    expect(attempts[0].get('[data-testid="chain-attempt-status"]').classes()).toContain('bg-amber-100')
    expect(attempts[3].get('[data-testid="chain-attempt-status"]').classes()).toContain('bg-red-100')
    expect(attempts[3].get('[data-testid="chain-attempt-status"]').text()).toBe('503')

    const final = wrapper.get('[data-testid="chain-final"]')
    expect(final.classes()).toContain('bg-red-50/70')
    expect(final.text()).toContain('✗')
    expect(final.get('[data-testid="chain-final-status"]').text()).toBe('admin.accounts.requestChain.finalFailed')
  })

  it('says so plainly when no successful record could be linked', async () => {
    getRequestChain.mockResolvedValue({ ...recoveredSameAccount, final: null })
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    expect(wrapper.get('[data-testid="chain-outcome-hint"]').text()).toBe('admin.accounts.requestChain.recoveredHintNoFinal|3')
    const final = wrapper.get('[data-testid="chain-final"]')
    expect(final.get('[data-testid="chain-final-missing"]').text()).toBe('admin.accounts.requestChain.finalMissing')
    expect(final.find('[data-testid="chain-final-account"]').exists()).toBe(false)
    expect(final.get('[data-testid="chain-final-metrics"]').text()).toBe('')
    // 说好恢复了却关联不到成功记录：数据对不上，用警示色而不是红色（免得误报成事故）
    expect(final.classes()).toContain('bg-amber-50/70')
  })

  // 后端的 account_id / account_name 都是 omitempty，关联不到账号时字段直接缺席
  it('degrades gracefully when an attempt has no account name', async () => {
    getRequestChain.mockResolvedValue({
      ...recoveredSameAccount,
      attempts: [
        { seq: 1, at: '2026-09-10T06:59:39Z', account_id: 7, platform: 'openai', upstream_status_code: 502, message: '', kind: 'overloaded' },
        { seq: 2, at: '2026-09-10T06:59:52Z', platform: 'openai', upstream_status_code: null, message: '', kind: '' }
      ]
    })
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    const attempts = wrapper.findAll('[data-testid="chain-attempt"]')
    expect(attempts[0].get('[data-testid="chain-attempt-account"]').text()).toBe('admin.accounts.requestChain.accountNoName|7')
    // kind 兜住空 message；两者都没有就画 '-'，不留空格
    expect(attempts[0].get('[data-testid="chain-attempt-message"]').text()).toBe('overloaded')
    expect(attempts[1].get('[data-testid="chain-attempt-account"]').text()).toBe('-')
    expect(attempts[1].get('[data-testid="chain-attempt-message"]').text()).toBe('-')
    // 没有上游状态码 ≠ 状态码 0，悬停说明清楚它压根没走到上游
    const missingStatus = attempts[1].get('[data-testid="chain-attempt-status"]')
    expect(missingStatus.text()).toBe('-')
    expect(missingStatus.attributes('title')).toBe('admin.accounts.requestChain.noUpstreamStatus')
    expect(attempts[0].get('[data-testid="chain-attempt-status"]').attributes('title')).toBeUndefined()
  })

  it('renders an explicit empty row when the chain has no attempts', async () => {
    getRequestChain.mockResolvedValue({ ...recoveredSameAccount, attempts: [] })
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    expect(wrapper.findAll('[data-testid="chain-attempt"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="chain-no-attempts"]').text()).toBe('admin.accounts.requestChain.noAttempts')
    expect(wrapper.get('[data-testid="chain-final"]').exists()).toBe(true)
  })

  it('shows a loading state instead of a blank panel, then the data', async () => {
    let resolveChain: (value: RequestChain) => void = () => {}
    getRequestChain.mockReturnValue(new Promise<RequestChain>(resolve => { resolveChain = resolve }))

    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    expect(wrapper.get('[data-testid="chain-loading"]').text()).toContain('common.loading')
    expect(wrapper.find('[data-testid="chain-outcome"]').exists()).toBe(false)

    resolveChain(recoveredSameAccount)
    await flushPromises()

    expect(wrapper.find('[data-testid="chain-loading"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="chain-outcome-title"]').text()).toBe('admin.accounts.requestChain.outcomeRecovered')
  })

  it('surfaces a retryable error instead of a blank panel when the request fails', async () => {
    getRequestChain.mockRejectedValue(new Error('404'))
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    expect(wrapper.get('[data-testid="chain-error"]').text()).toContain('admin.accounts.requestChain.loadFailed')
    expect(wrapper.find('[data-testid="chain-outcome"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="chain-loading"]').exists()).toBe(false)

    getRequestChain.mockResolvedValue(recoveredSameAccount)
    await wrapper.get('[data-testid="chain-retry"]').trigger('click')
    await flushPromises()

    expect(getRequestChain).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="chain-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="chain-attempt"]')).toHaveLength(3)
  })

  it('refetches when a different request id is opened', async () => {
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    getRequestChain.mockResolvedValue(failedChain)
    await wrapper.setProps({ clientRequestId: 'req-failed' })
    await flushPromises()

    expect(getRequestChain).toHaveBeenCalledTimes(2)
    expect(getRequestChain).toHaveBeenLastCalledWith('req-failed')
    expect(wrapper.get('[data-testid="chain-outcome-title"]').text()).toBe('admin.accounts.requestChain.outcomeFailed')
  })

  it('gives the account name and the time room, squeezes seq/status, and never overflows a phone', async () => {
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    // 宽度：窄屏收在 96vw 内（不溢出），大屏至少 760px 并跟到 44vw
    const width = wrapper.findComponent(SideDrawerStub).props('width') as string
    expect(width).toBe('min(96vw, max(44vw, 760px))')
    expect(width).toMatch(/min\(\s*9\d?vw/)

    const rowClasses = wrapper.get('[data-testid="chain-attempt"]').classes()
    expect(rowClasses).toContain('grid')
    // 窄屏 4 列：序号 / 时间 / 账号（吃满剩余） / 状态码；错误信息掉到第二行
    expect(rowClasses).toContain('grid-cols-[1.25rem_4.5rem_minmax(0,1fr)_auto]')
    // 宽屏 5 列：账号 minmax(7rem,7fr) 与信息 9fr 分剩余宽度，序号/状态码钉死在最窄
    expect(rowClasses).toContain('sm:grid-cols-[1.25rem_5rem_minmax(7rem,7fr)_3.25rem_minmax(0,9fr)]')
    expect(wrapper.get('[data-testid="chain-final"]').classes()).toContain('grid-cols-[1.25rem_4.5rem_minmax(0,1fr)_auto]')

    // 时间不换行、账号放不下才截断并用 title 看全名
    const time = wrapper.get('[data-testid="chain-attempt"] span:nth-child(2)')
    expect(time.classes()).toContain('whitespace-nowrap')
    const account = wrapper.get('[data-testid="chain-attempt-account"]')
    expect(account.classes()).toContain('truncate')
    expect(account.classes()).toContain('min-w-0')
    expect(account.attributes('title')).toBe('admin.accounts.requestChain.account|7,anvizanviz80')

    // 错误信息是次要信息：截断 + title，窄屏整行独占，宽屏回到第 5 列
    const message = wrapper.get('[data-testid="chain-attempt-message"]')
    expect(message.classes()).toContain('truncate')
    expect(message.classes()).toContain('col-span-4')
    expect(message.classes()).toContain('sm:col-span-1')
    expect(message.attributes('title')).toBe('上游过载')
  })

  it('forwards close from the drawer shell', async () => {
    const wrapper = mountPanel({ show: true, clientRequestId: 'req-recovered' })
    await flushPromises()

    wrapper.findComponent(SideDrawerStub).vm.$emit('close')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
