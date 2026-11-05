import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ErrorKindSummary from '../ErrorKindSummary.vue'
import type { OpsErrorLogSummaryResponse } from '@/api/admin/ops'

const messages: Record<string, string> = {
  'admin.usage.errorSummary.title': 'Error Types',
  'admin.usage.errorSummary.total': '{n} total',
  'admin.usage.errorSummary.all': 'All',
  'admin.usage.errorSummary.share': '{percent}% of current errors',
  'admin.usage.errorSummary.other': 'Other',
  'admin.usage.errorSummary.otherUnavailable': 'Other types cannot be filtered individually',
  'admin.usage.errorSummary.reasons.modelRateLimited': 'All accounts are rate limited',
  'admin.usage.errorSummary.loadFailed': 'Failed to load error types',
  'admin.usage.errorSummary.retry': 'Retry',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        const message = messages[key] ?? key
        return Object.entries(params ?? {}).reduce(
          (value, [name, replacement]) => value.replaceAll(`{${name}}`, replacement),
          message
        )
      },
    }),
  }
})

const summary: OpsErrorLogSummaryResponse = {
  total: 100,
  items: [
    {
      kind: 'upstream|529|overloaded',
      phase: 'upstream',
      status_code: 529,
      reason: '',
      sample_message: 'Our servers are currently overloaded',
      count: 20,
      failed: 0,
      recovered: 20,
    },
    {
      kind: 'routing|503|model_rate_limited',
      phase: 'routing',
      status_code: 503,
      reason: 'model_rate_limited',
      sample_message: 'Service temporarily unavailable',
      count: 70,
      failed: 70,
      recovered: 0,
    },
    {
      kind: 'other',
      phase: '',
      status_code: 0,
      reason: '',
      sample_message: '',
      count: 10,
      failed: 10,
      recovered: 0,
    },
  ],
}

function mountSummary(
  selectedKind: string | null = null,
  overrides: Partial<{
    summary: OpsErrorLogSummaryResponse | null
    loading: boolean
    error: boolean
  }> = {}
) {
  return mount(ErrorKindSummary, {
    props: {
      summary: overrides.summary === undefined ? summary : overrides.summary,
      loading: false,
      error: false,
      selectedKind,
      ...overrides,
    },
  })
}

describe('ErrorKindSummary', () => {
  it('sorts cards by count descending and shows reason labels', () => {
    const wrapper = mountSummary()
    const cards = wrapper.findAll('[data-testid="error-kind-card"]')

    expect(cards.map((card) => card.attributes('data-kind'))).toEqual([
      'routing|503|model_rate_limited',
      'upstream|529|overloaded',
      'other',
    ])
    expect(cards[0].text()).toContain('All accounts are rate limited')
    expect(cards[0].text()).toContain('70')
    expect(wrapper.find('[data-testid="error-kind-total"]').text()).toBe('100 total')
  })

  it('emits selection and clears it when the selected card is clicked again', async () => {
    const wrapper = mountSummary()
    const card = wrapper.find('[data-kind="routing|503|model_rate_limited"]')

    await card.trigger('click')
    expect(wrapper.emitted('select')).toEqual([['routing|503|model_rate_limited']])

    await wrapper.setProps({ selectedKind: 'routing|503|model_rate_limited' })
    await card.trigger('click')
    expect(wrapper.emitted('select')?.at(-1)).toEqual([null])
  })

  it('does not allow the synthetic other bucket to be selected', async () => {
    const wrapper = mountSummary()
    const other = wrapper.find('[data-kind="other"]')

    expect(other.attributes('disabled')).toBeDefined()
    await other.trigger('click')
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('emits all selection from the reset button', async () => {
    const wrapper = mountSummary('routing|503|model_rate_limited')

    await wrapper.find('[data-testid="error-kind-all"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[null]])
  })

  it('shows the loading skeleton while the summary request is pending', () => {
    const wrapper = mountSummary(null, { summary: null, loading: true })

    expect(wrapper.find('[data-testid="error-kind-summary-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="error-kind-card"]').exists()).toBe(false)
  })

  it('shows a retry action when loading fails', async () => {
    const wrapper = mountSummary(null, { summary: null, error: true })

    expect(wrapper.find('[data-testid="error-kind-summary-error"]').text()).toContain('Failed to load error types')
    await wrapper.find('[data-testid="error-kind-summary-error"] button').trigger('click')
    expect(wrapper.emitted('retry')).toEqual([[]])
  })

  it('renders nothing when the filtered summary is empty', () => {
    const wrapper = mountSummary(null, {
      summary: { total: 0, items: [] },
      loading: false,
      error: false,
    })

    expect(wrapper.find('[data-testid="error-kind-summary"]').exists()).toBe(false)
  })

  it('shows status code plus a 40-character message excerpt when reason is absent', () => {
    const message = 'This message is intentionally longer than forty characters to prove truncation'
    const wrapper = mountSummary(null, {
      summary: {
        total: 1,
        items: [
          {
            kind: 'request|400|long-message',
            phase: 'request',
            status_code: 400,
            reason: '',
            sample_message: message,
            count: 1,
            failed: 1,
            recovered: 0,
          },
        ],
      },
    })

    const expected = `400 ${message.slice(0, 40)}…`
    const cardText = wrapper.find('[data-testid="error-kind-card"]').text()
    expect(cardText).toContain(expected)
    expect(cardText).not.toContain(message)
  })

  it('shows an unknown reason code verbatim', () => {
    const wrapper = mountSummary(null, {
      summary: {
        total: 1,
        items: [
          {
            kind: 'routing|403|vendor_specific_reason',
            phase: 'routing',
            status_code: 403,
            reason: 'vendor_specific_reason',
            sample_message: 'Forbidden',
            count: 1,
            failed: 1,
            recovered: 0,
          },
        ],
      },
    })

    expect(wrapper.find('[data-testid="error-kind-card"]').text()).toContain('vendor_specific_reason')
  })
})
