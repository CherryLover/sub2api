import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AccountRecentErrors } from '@/types'

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
    formatRelativeTime: () => '5m ago',
    formatDateTime: (value: string) => `abs:${value}`
  }
})

import AccountRecentErrorsCell from '../AccountRecentErrorsCell.vue'

function mountCell(props: { errors?: AccountRecentErrors | null; loading?: boolean }) {
  return mount(AccountRecentErrorsCell, {
    props,
    global: {
      // HelpTooltip teleports its bubble to <body>; render it inline so the text is assertable
      stubs: { teleport: true }
    }
  })
}

const LAST_AT = '2026-09-10T08:58:00Z'

describe('AccountRecentErrorsCell', () => {
  beforeEach(() => {
    getRequestChain.mockReset().mockResolvedValue({
      client_request_id: 'req-abc',
      created_at: LAST_AT,
      model: 'gpt-5.6-sol',
      requested_model: 'gpt-5.6-sol',
      stream: true,
      outcome: 'recovered',
      client_status_code: 200,
      attempts: [],
      final: null
    })
  })

  // 「取不到数据」必须和「确实零错误」长得不一样：都画成 `-` 的话，
  // 运维监控一关，整列看起来就像全站零错误。
  it('distinguishes unavailable (null) from genuinely zero errors', () => {
    const wrapper = mountCell({ errors: null })
    expect(wrapper.find('[data-test="recent-errors-unavailable"]').text()).toBe('?')
    expect(wrapper.find('[data-test="recent-errors-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="recent-errors"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.recentErrors.unavailable')
  })

  it('shows a dash when there are no errors in the window', () => {
    const wrapper = mountCell({ errors: { total: 0, by_status: [], last: null } })
    expect(wrapper.find('[data-test="recent-errors-empty"]').text()).toBe('-')
  })

  it('shows a skeleton only while the first load is pending', () => {
    const pending = mountCell({ errors: null, loading: true })
    expect(pending.find('.animate-pulse').exists()).toBe(true)

    const refreshing = mountCell({ errors: { total: 0, by_status: [], last: null }, loading: true })
    expect(refreshing.find('.animate-pulse').exists()).toBe(false)
    expect(refreshing.text()).toBe('-')
  })

  it('renders up to 3 status chips sorted by count, colored by status class', () => {
    const wrapper = mountCell({
      errors: {
        total: 23,
        by_status: [
          { status_code: 429, count: 4 },
          { status_code: 400, count: 2 },
          { status_code: 502, count: 14 },
          { status_code: 503, count: 3 }
        ],
        last: { at: LAST_AT, status_code: 502, message: 'bad gateway' }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips.map(chip => chip.text())).toEqual(['502 ×14', '429 ×4', '503 ×3'])
    expect(chips[0].classes()).toContain('bg-red-100')
    expect(chips[1].classes()).toContain('bg-amber-100')
    expect(chips[2].classes()).toContain('bg-red-100')
    // 第 4 个状态码被折叠
    expect(wrapper.text()).toContain('+1')
    expect(wrapper.text()).not.toContain('400 ×2')
  })

  it('uses gray chips for non-429 4xx codes', () => {
    const wrapper = mountCell({
      errors: {
        total: 2,
        by_status: [{ status_code: 400, count: 2 }],
        last: { at: LAST_AT, status_code: 400 }
      }
    })
    expect(wrapper.find('[data-test="recent-errors-chip"]').classes()).toContain('bg-gray-100')
  })

  it('shows the relative time of the latest error on the second line', () => {
    const wrapper = mountCell({
      errors: {
        total: 4,
        by_status: [{ status_code: 429, count: 4 }],
        last: { at: LAST_AT, status_code: 429 }
      }
    })
    expect(wrapper.find('[data-test="recent-errors"]').text()).toContain('admin.accounts.recentErrors.lastAt|5m ago')
  })

  it('tooltip shows absolute time, upstream status, model and the full message', () => {
    const message = 'Our servers are currently overloaded. Please try again later.'
    const wrapper = mountCell({
      errors: {
        total: 18,
        by_status: [
          { status_code: 502, count: 14 },
          { status_code: 429, count: 4 }
        ],
        last: {
          at: LAST_AT,
          status_code: 200,
          upstream_status_code: 502,
          message,
          model: 'gpt-5.6-sol'
        }
      }
    })

    const tooltip = wrapper.find('[data-test="recent-errors-tooltip"]')
    expect(tooltip.exists()).toBe(true)
    expect(tooltip.text()).toContain(`abs:${LAST_AT}`)
    expect(tooltip.text()).toContain('admin.accounts.recentErrors.upstreamStatus|502')
    expect(tooltip.text()).not.toContain('200')
    expect(tooltip.text()).toContain('gpt-5.6-sol')
    expect(tooltip.text()).toContain(message)
  })

  it('tooltip falls back to the gateway status code when there is no upstream code', () => {
    const wrapper = mountCell({
      errors: {
        total: 1,
        by_status: [{ status_code: 429, count: 1 }],
        last: { at: LAST_AT, status_code: 429, upstream_status_code: null }
      }
    })

    const tooltip = wrapper.find('[data-test="recent-errors-tooltip"]')
    expect(tooltip.text()).toContain('429')
    expect(tooltip.text()).not.toContain('admin.accounts.recentErrors.upstreamStatus')
  })

  // ---------------------------------------------------------------------------
  // 已恢复 / 最终失败
  // `502 ×5` 会让站长以为出了五次事故，实际五次都被网关自己救回来了。
  // ---------------------------------------------------------------------------

  it('renders auto-recovered errors in a muted amber chip, never red', () => {
    const wrapper = mountCell({
      errors: {
        total: 5,
        recovered: 5,
        failed: 0,
        by_status: [{ status_code: 502, count: 5, recovered: 5, failed: 0 }],
        last: { at: LAST_AT, status_code: 200, upstream_status_code: 502 }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips).toHaveLength(1)
    expect(chips[0].text()).toBe('admin.accounts.recentErrors.chipRecovered|502,5')
    expect(chips[0].classes()).toContain('bg-amber-50')
    expect(chips[0].classes()).not.toContain('bg-red-100')
  })

  it('renders errors the client actually saw in a red chip', () => {
    const wrapper = mountCell({
      errors: {
        total: 3,
        recovered: 0,
        failed: 3,
        by_status: [{ status_code: 502, count: 3, recovered: 0, failed: 3 }],
        last: { at: LAST_AT, status_code: 502, upstream_status_code: 502 }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips).toHaveLength(1)
    expect(chips[0].text()).toBe('admin.accounts.recentErrors.chipFailed|502,3')
    expect(chips[0].classes()).toContain('bg-red-100')
  })

  it('splits one status code into separate recovered and failed chips, failures first', () => {
    const wrapper = mountCell({
      errors: {
        total: 6,
        recovered: 5,
        failed: 1,
        by_status: [{ status_code: 502, count: 6, recovered: 5, failed: 1 }],
        last: { at: LAST_AT, status_code: 502, upstream_status_code: 502 }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips.map(chip => chip.text())).toEqual([
      'admin.accounts.recentErrors.chipFailed|502,1',
      'admin.accounts.recentErrors.chipRecovered|502,5'
    ])
    // 即便"已恢复"的次数更多，最终失败仍排在最前：那才是需要站长看一眼的
    expect(chips[0].classes()).toContain('bg-red-100')
    expect(chips[1].classes()).toContain('bg-amber-50')
  })

  it('sorts failures ahead of recoveries across status codes and collapses the overflow', () => {
    const wrapper = mountCell({
      errors: {
        total: 20,
        recovered: 18,
        failed: 2,
        by_status: [
          { status_code: 502, count: 14, recovered: 14, failed: 0 },
          { status_code: 429, count: 4, recovered: 4, failed: 0 },
          { status_code: 529, count: 2, recovered: 0, failed: 2 }
        ],
        last: { at: LAST_AT, status_code: 529, upstream_status_code: 529 }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips.map(chip => chip.text())).toEqual([
      'admin.accounts.recentErrors.chipFailed|529,2',
      'admin.accounts.recentErrors.chipRecovered|502,14',
      'admin.accounts.recentErrors.chipRecovered|429,4'
    ])
    expect(wrapper.text()).not.toContain('+')
  })

  it('backfills the missing side when the backend only reports one of recovered/failed', () => {
    const wrapper = mountCell({
      errors: {
        total: 4,
        by_status: [{ status_code: 500, count: 4, failed: 1 }],
        last: { at: LAST_AT, status_code: 500 }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips.map(chip => chip.text())).toEqual([
      'admin.accounts.recentErrors.chipFailed|500,1',
      'admin.accounts.recentErrors.chipRecovered|500,3'
    ])
  })

  // 后端无条件序列化 recovered/failed，拆分能力缺失时下发的是 0 而不是缺字段。
  // 把 0/0 当成"真的零恢复零失败"会让整格 chip 消失，比显示旧口径还糟。
  it('falls back to the legacy chip when the backend reports 0 recovered and 0 failed', () => {
    const wrapper = mountCell({
      errors: {
        total: 4,
        recovered: 0,
        failed: 0,
        by_status: [{ status_code: 502, count: 4, recovered: 0, failed: 0 }],
        last: { at: LAST_AT, status_code: 502 }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    expect(chips).toHaveLength(1)
    expect(chips[0].text()).toBe('502 ×4')
    expect(chips[0].classes()).toContain('bg-red-100')
    expect(wrapper.find('[data-test="recent-errors-summary"]').exists()).toBe(false)
  })

  it('keeps the legacy total-only chip when by_status is empty', () => {
    const wrapper = mountCell({
      errors: { total: 7, by_status: [], last: { at: LAST_AT, status_code: 502 } }
    })
    expect(wrapper.find('[data-test="recent-errors-chip"]').text()).toBe('×7')
  })

  it('splits the total-only chip by outcome when the backend reports one', () => {
    const wrapper = mountCell({
      errors: { total: 7, recovered: 6, failed: 1, by_status: [], last: { at: LAST_AT, status_code: 502 } }
    })
    expect(wrapper.findAll('[data-test="recent-errors-chip"]').map(chip => chip.text())).toEqual([
      'admin.accounts.recentErrors.chipTotalFailed|1',
      'admin.accounts.recentErrors.chipTotalRecovered|6'
    ])
  })

  it('explains in the tooltip that "recovered" means the gateway healed it by itself', () => {
    const wrapper = mountCell({
      errors: {
        total: 6,
        recovered: 5,
        failed: 1,
        by_status: [{ status_code: 502, count: 6, recovered: 5, failed: 1 }],
        last: { at: LAST_AT, status_code: 502, client_request_id: 'req-abc' }
      }
    })

    const tooltip = wrapper.find('[data-test="recent-errors-tooltip"]')
    expect(tooltip.find('[data-test="recent-errors-summary"]').text()).toContain('admin.accounts.recentErrors.summaryTotal|6')
    expect(tooltip.find('[data-test="recent-errors-summary"]').text()).toContain('admin.accounts.recentErrors.summaryRecovered|5')
    expect(tooltip.find('[data-test="recent-errors-summary"]').text()).toContain('admin.accounts.recentErrors.summaryFailed|1')
    expect(tooltip.find('[data-test="recent-errors-recovered-hint"]').text()).toBe('admin.accounts.recentErrors.recoveredExplainer')
    expect(tooltip.find('[data-test="recent-errors-chain-hint"]').text()).toBe('admin.accounts.recentErrors.chainHint')
  })

  it('omits the recovered explainer for legacy payloads without the outcome split', () => {
    const wrapper = mountCell({
      errors: {
        total: 2,
        by_status: [{ status_code: 502, count: 2 }],
        last: { at: LAST_AT, status_code: 502 }
      }
    })
    expect(wrapper.find('[data-test="recent-errors-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="recent-errors-recovered-hint"]').exists()).toBe(false)
  })

  // ---------------------------------------------------------------------------
  // 请求链路入口
  // ---------------------------------------------------------------------------

  it('opens the request chain panel for the clicked chip and asks for that client_request_id', async () => {
    const wrapper = mountCell({
      errors: {
        total: 5,
        recovered: 5,
        failed: 0,
        by_status: [{ status_code: 502, count: 5, recovered: 5, failed: 0, last_client_request_id: 'req-chain-1' }],
        last: { at: LAST_AT, status_code: 200, upstream_status_code: 502, client_request_id: 'req-other' }
      }
    })

    const chip = wrapper.find('[data-test="recent-errors-chip"]')
    expect(chip.element.tagName).toBe('BUTTON')

    await chip.trigger('click')
    await flushPromises()

    expect(getRequestChain).toHaveBeenCalledTimes(1)
    expect(getRequestChain).toHaveBeenCalledWith('req-chain-1')
    expect(wrapper.find('[data-testid="chain-request-id"]').text()).toBe('req-chain-1')
  })

  it('falls back to the latest error id only for the status bucket that error belongs to', async () => {
    const wrapper = mountCell({
      errors: {
        total: 5,
        recovered: 4,
        failed: 1,
        by_status: [
          { status_code: 502, count: 4, recovered: 4, failed: 0 },
          { status_code: 429, count: 1, recovered: 0, failed: 1 }
        ],
        last: { at: LAST_AT, status_code: 200, upstream_status_code: 429, client_request_id: 'req-429' }
      }
    })

    const chips = wrapper.findAll('[data-test="recent-errors-chip"]')
    // 429 是 last 所属的档，可点；502 那档后端没给 id，保持纯展示
    expect(chips[0].text()).toBe('admin.accounts.recentErrors.chipFailed|429,1')
    expect(chips[0].element.tagName).toBe('BUTTON')
    expect(chips[1].element.tagName).toBe('SPAN')

    await chips[0].trigger('click')
    await flushPromises()
    expect(getRequestChain).toHaveBeenCalledWith('req-429')
  })

  it('stays a plain chip and never opens a panel when the backend sends no client_request_id', async () => {
    const wrapper = mountCell({
      errors: {
        total: 5,
        recovered: 5,
        failed: 0,
        by_status: [{ status_code: 502, count: 5, recovered: 5, failed: 0 }],
        last: { at: LAST_AT, status_code: 200, upstream_status_code: 502 }
      }
    })

    const chip = wrapper.find('[data-test="recent-errors-chip"]')
    expect(chip.element.tagName).toBe('SPAN')
    expect(wrapper.find('[data-test="recent-errors-chain-hint"]').exists()).toBe(false)

    await chip.trigger('click')
    await flushPromises()
    expect(getRequestChain).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="chain-request-id"]').exists()).toBe(false)
  })
})
