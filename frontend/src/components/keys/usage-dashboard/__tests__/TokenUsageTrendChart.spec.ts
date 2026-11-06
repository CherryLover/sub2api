import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import TokenUsageTrendChart from '../TokenUsageTrendChart.vue'
import { createDashboardKey, metric } from './fixtures'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        if (key === 'keyUsageDashboard.trend.other') return 'Other keys'
        if (key === 'keyUsageDashboard.metrics.cost') return 'Cost'
        return key
      }
    })
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<div data-testid="line-chart">{{ JSON.stringify(data) }}</div>'
  }
}))

describe('TokenUsageTrendChart', () => {
  it('converts the leading five keys and aggregates the rest into Other', () => {
    const keys = Array.from({ length: 7 }, (_, index) => {
      const id = index + 1
      return createDashboardKey(id, {
        totals: metric(id * 10, id * 100, id * 1000),
        series: [metric(id, id, id), metric(id * 2, id * 2, id * 2)]
      })
    })

    const wrapper = mount(TokenUsageTrendChart, {
      props: {
        keys,
        seriesLabels: ['11/05', '11/06'],
        metric: 'cost'
      }
    })

    const chartData = JSON.parse(wrapper.get('[data-testid="line-chart"]').text())
    expect(chartData.labels).toEqual(['11/05', '11/06'])
    expect(chartData.datasets.map((dataset: { label: string }) => dataset.label)).toEqual([
      'key-7',
      'key-6',
      'key-5',
      'key-4',
      'key-3',
      'Other keys'
    ])
    expect(chartData.datasets[5].data).toEqual([3, 6])
    expect(chartData.datasets[5].borderColor).toBe('#94a3b8')
    expect(chartData.datasets.every((dataset: { fill: boolean }) => dataset.fill)).toBe(true)
    expect(chartData.datasets.every((dataset: { stack: string }) => dataset.stack === 'usage')).toBe(true)
  })
})
