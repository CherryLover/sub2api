import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageHeatmap from '../UsageHeatmap.vue'
import { createDashboardKey } from './fixtures'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const message = key === 'keyUsageDashboard.heatmap.showAll'
          ? 'Show all {total} ({idle} idle)'
          : key
        return params
          ? message.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name]))
          : message
      }
    })
  }
})

function mountHeatmap() {
  const keys = Array.from({ length: 20 }, (_, index) => createDashboardKey(index + 1))
  keys[19].name = 'needle-key'
  return mount(UsageHeatmap, {
    props: {
      keys,
      seriesLabels: ['10/31', '11/01', '11/02', '11/03', '11/04', '11/05', '11/06'],
      range: '7d',
      metric: 'requests',
      selectedKeyId: 20
    }
  })
}

describe('UsageHeatmap', () => {
  it('shows the first 15 keys and expands to all keys', async () => {
    const wrapper = mountHeatmap()

    expect(wrapper.findAll('tbody tr')).toHaveLength(15)
    const expand = wrapper.get('[data-testid="usage-heatmap-expand"]')
    expect(expand.text()).toContain('Show all 20')

    await expand.trigger('click')

    expect(wrapper.findAll('tbody tr')).toHaveLength(20)
    expect(wrapper.get('[data-testid="usage-heatmap-expand"]').text()).toBe('keyUsageDashboard.heatmap.collapse')
  })

  it('filters keys and shows a selected cell value below the heatmap', async () => {
    const wrapper = mountHeatmap()

    await wrapper.get('[data-testid="usage-heatmap-search"]').setValue('needle')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.text()).toContain('needle-key')

    await wrapper.get('[data-testid="usage-cell-20-11"]').trigger('click')

    const detail = wrapper.get('[data-testid="usage-heatmap-cell-detail"]').text()
    expect(detail).toContain('needle-key')
    expect(detail).toContain('11:00')
    expect(detail).toContain('20')
  })

  it('emits the selected key when its name is clicked', async () => {
    const wrapper = mountHeatmap()

    const selectedRow = wrapper.findAll('tbody th button').find((button) =>
      button.text().includes('needle-key')
    )
    expect(selectedRow).toBeDefined()
    await selectedRow!.trigger('click')

    expect(wrapper.emitted('select')?.[0]).toEqual([20])
  })
})
