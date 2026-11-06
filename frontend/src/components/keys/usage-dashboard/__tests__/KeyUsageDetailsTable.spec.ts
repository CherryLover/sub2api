import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import KeyUsageDetailsTable from '../KeyUsageDetailsTable.vue'
import { createDashboardKey, metric } from './fixtures'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function rowIds(wrapper: ReturnType<typeof mount>): string[] {
  return wrapper.findAll('tbody tr')
    .map((row) => row.attributes('data-testid') || '')
    .filter(Boolean)
}

describe('KeyUsageDetailsTable', () => {
  it('sorts by the current metric and toggles the same column in reverse order', async () => {
    const wrapper = mount(KeyUsageDetailsTable, {
      props: {
        metric: 'requests',
        keys: [
          createDashboardKey(1, { totals: metric(4, 40, 400) }),
          createDashboardKey(2, { totals: metric(2, 20, 200) }),
          createDashboardKey(3, { totals: metric(1, 10, 100) })
        ]
      }
    })

    expect(rowIds(wrapper)).toEqual([
      'usage-detail-row-1',
      'usage-detail-row-2',
      'usage-detail-row-3'
    ])

    const requestsSort = wrapper.get('[data-testid="usage-sort-requests"]')
    expect(requestsSort.element.closest('th')?.getAttribute('aria-sort')).toBe('descending')

    await requestsSort.trigger('click')

    expect(rowIds(wrapper)).toEqual([
      'usage-detail-row-3',
      'usage-detail-row-2',
      'usage-detail-row-1'
    ])
    expect(requestsSort.element.closest('th')?.getAttribute('aria-sort')).toBe('ascending')
  })

  it('uses the correct aria-sort state and red/amber error emphasis', () => {
    const wrapper = mount(KeyUsageDetailsTable, {
      props: {
        metric: 'cost',
        keys: [
          createDashboardKey(1, { totals: metric(5, 100, 1000), error_rate: 0.06 }),
          createDashboardKey(2, { totals: metric(3, 80, 800), error_rate: 0.03 }),
          createDashboardKey(3, { totals: metric(0, 0, 0) })
        ]
      }
    })

    expect(wrapper.get('[data-testid="usage-sort-cost"]').element.closest('th')?.getAttribute('aria-sort')).toBe('descending')
    expect(wrapper.get('[data-testid="usage-sort-requests"]').element.closest('th')?.getAttribute('aria-sort')).toBe('none')
    expect(wrapper.get('[data-testid="usage-detail-row-1"]').html()).toContain('bg-red-50')
    expect(wrapper.get('[data-testid="usage-detail-row-2"]').html()).toContain('bg-amber-50')
    expect(wrapper.get('[data-testid="usage-detail-row-3"]').classes()).toContain('opacity-60')
    expect(wrapper.get('[data-testid="usage-detail-row-3"]').text()).toContain('—')
  })
})
