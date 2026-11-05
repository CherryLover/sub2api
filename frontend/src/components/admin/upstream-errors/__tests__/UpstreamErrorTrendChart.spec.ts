import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UpstreamErrorTrendChart from '../UpstreamErrorTrendChart.vue'

const { getTrend } = vi.hoisted(() => ({ getTrend: vi.fn() }))

vi.mock('@/api/admin/ops', () => ({
  getUpstreamErrorRequestsTrend: getTrend
}))

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('vue-chartjs', async () => {
  const { defineComponent } = await import('vue')
  return {
    Line: defineComponent({
      name: 'ChartLine',
      props: ['data', 'options'],
      template: '<div class="line-stub">{{ JSON.stringify(data) }}</div>'
    })
  }
})

const trendResponse = {
  days: [
    { date: '2026-11-04', total: 5, recovered: 3, failed: 2 },
    { date: '2026-11-05', total: 8, recovered: 5, failed: 3 }
  ]
}

describe('UpstreamErrorTrendChart', () => {
  beforeEach(() => {
    getTrend.mockReset().mockResolvedValue(trendResponse)
  })

  it('loads 14 days by default and renders the three line series', async () => {
    const wrapper = mount(UpstreamErrorTrendChart, {
      props: { filters: {} },
      global: { stubs: { LoadingSpinner: true } }
    })
    await flushPromises()

    expect(getTrend).toHaveBeenCalledWith(
      { days: 14 },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    const chartData = JSON.parse(wrapper.get('.line-stub').text())
    expect(chartData.datasets).toHaveLength(3)
    expect(chartData.datasets[0].borderColor).toBe('#475569')
    expect(chartData.datasets[1].borderColor).toBe('#0d9488')
    expect(chartData.datasets[2].borderColor).toBe('#dc2626')
    expect(chartData.datasets[2].borderDash).toEqual([6, 4])
    wrapper.unmount()
  })

  it('requests the selected 7/14/30 day window', async () => {
    const wrapper = mount(UpstreamErrorTrendChart, {
      props: { filters: {} },
      global: { stubs: { LoadingSpinner: true } }
    })
    await flushPromises()

    await wrapper.get('[data-testid="trend-days-30"]').trigger('click')
    await flushPromises()
    expect(getTrend.mock.calls.at(-1)![0]).toMatchObject({ days: 30 })

    await wrapper.get('[data-testid="trend-days-7"]').trigger('click')
    await flushPromises()
    expect(getTrend.mock.calls.at(-1)![0]).toMatchObject({ days: 7 })
    wrapper.unmount()
  })

  it('refreshes when non-time filters change and ignores stale responses', async () => {
    let resolveOld!: (value: typeof trendResponse) => void
    getTrend.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mount(UpstreamErrorTrendChart, {
      props: { filters: {} },
      global: { stubs: { LoadingSpinner: true } }
    })

    await wrapper.setProps({ filters: { status_code: 429 } })
    await flushPromises()
    expect(getTrend.mock.calls.at(-1)![0]).toMatchObject({ status_code: 429, days: 14 })

    resolveOld({
      days: [{ date: '2026-11-03', total: 99, recovered: 0, failed: 99 }]
    })
    await flushPromises()
    expect(wrapper.get('.line-stub').text()).toContain('8')
    expect(wrapper.get('.line-stub').text()).not.toContain('99')
    wrapper.unmount()
  })
})
