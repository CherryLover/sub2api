import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountRecentErrorsCell from '../AccountRecentErrorsCell.vue'
import type { AccountRecentErrors } from '@/types'

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
})
