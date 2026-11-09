import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'

import UsageDashboard from '../UsageDashboard.vue'
import CurrentKeyPanel from '../CurrentKeyPanel.vue'
import { createDashboardFixture, metric } from './fixtures'

const { getDashboard, getDetail, getGroups } = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  getDetail: vi.fn(),
  getGroups: vi.fn()
}))

vi.mock('@/api/keyUsageDashboard', async () => {
  const actual = await vi.importActual<typeof import('@/api/keyUsageDashboard')>(
    '@/api/keyUsageDashboard'
  )
  return {
    ...actual,
    getKeyUsageDashboard: getDashboard,
    getKeyUsageDashboardDetail: getDetail
  }
})

vi.mock('@/api', () => ({
  userGroupsAPI: {
    getAvailable: getGroups
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        if (key === 'keyUsageDashboard.metrics.cost') return 'Cost'
        if (key === 'keyUsageDashboard.metrics.requests') return 'Requests'
        return key
      }
    })
  }
})

const CurrentKeyPanelStub = defineComponent({
  name: 'CurrentKeyPanel',
  props: ['apiKeyId'],
  template: '<div data-testid="current-key-stub">{{ apiKeyId }}</div>'
})

const UsageAlertsStub = defineComponent({
  name: 'UsageAlerts',
  props: ['alerts'],
  emits: ['select'],
  template: '<button data-testid="alert-select" @click="$emit(\'select\', 2)">alert</button>'
})

const UsageHeatmapStub = defineComponent({
  name: 'UsageHeatmap',
  props: ['selectedKeyId'],
  emits: ['select'],
  template: '<button data-testid="heatmap-select" @click="$emit(\'select\', 3)">heatmap</button>'
})

function mountDashboard(initialKeyId = 2): VueWrapper {
  return mount(UsageDashboard, {
    props: {
      initialKeyId
    },
    global: {
      stubs: {
        UsageStatsCards: true,
        UsageAlerts: UsageAlertsStub,
        UsageHeatmap: UsageHeatmapStub,
        CurrentKeyPanel: CurrentKeyPanelStub,
        TokenUsageTrendChart: true,
        WeeklyUsageHeatmap: true,
        KeyModelHeatmap: true,
        KeyUsageDetailsTable: true
      }
    }
  })
}

describe('UsageDashboard', () => {
  beforeEach(() => {
    getDashboard.mockReset()
    getDetail.mockReset()
    getGroups.mockReset()
    getGroups.mockResolvedValue([{ id: 7, name: 'default' }])
    getDashboard.mockResolvedValue(createDashboardFixture({
      alerts: [{
        type: 'usage_spike',
        api_key_id: 2,
        api_key_name: 'key-2',
        value: 8.5,
        count: 0
      }]
    }))
    getDetail.mockResolvedValue({
      api_key_id: 2,
      name: 'key-2',
      group_name: 'default',
      totals: metric(2, 20, 200),
      cache_hit_rate: 0.4,
      avg_first_token_ms: 800,
      error_rate: 0.02,
      peak_rpm: 4,
      large_request_count: 0,
      large_request_avg_bytes: 0,
      hour_of_day: Array.from({ length: 24 }, () => metric()),
      models: [],
      efforts: []
    })
  })

  it('refetches when range changes but keeps metric changes client-side', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(getDashboard).toHaveBeenCalledTimes(1)
    expect(getDashboard).toHaveBeenLastCalledWith(
      { range: '7d' },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )

    await wrapper.get('[data-testid="key-usage-metric-requests"]').trigger('click')
    await flushPromises()
    expect(getDashboard).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="key-usage-range-today"]').trigger('click')
    await flushPromises()
    expect(getDashboard).toHaveBeenCalledTimes(2)
    expect(getDashboard).toHaveBeenLastCalledWith(
      { range: 'today' },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('selects a key from alerts and heatmap cells', async () => {
    const wrapper = mountDashboard(1)
    await flushPromises()

    expect(wrapper.get('[data-testid="current-key-stub"]').text()).toBe('1')
    await wrapper.get('[data-testid="alert-select"]').trigger('click')
    expect(wrapper.get('[data-testid="current-key-stub"]').text()).toBe('2')

    await wrapper.get('[data-testid="heatmap-select"]').trigger('click')
    expect(wrapper.get('[data-testid="current-key-stub"]').text()).toBe('3')
  })
})

describe('CurrentKeyPanel', () => {
  beforeEach(() => {
    getDetail.mockReset()
    getDetail.mockResolvedValue({
      api_key_id: 3,
      name: 'key-3',
      group_name: 'default',
      totals: metric(3, 30, 300),
      cache_hit_rate: 0.5,
      avg_first_token_ms: 700,
      error_rate: 0.01,
      peak_rpm: 6,
      large_request_count: 0,
      large_request_avg_bytes: 0,
      hour_of_day: Array.from({ length: 24 }, () => metric()),
      models: [],
      efforts: []
    })
  })

  it('loads details when a key is selected', async () => {
    mount(CurrentKeyPanel, {
      props: {
        apiKeyId: 3,
        keyName: 'key-3',
        params: { range: '7d' },
        metric: 'cost'
      },
      global: {
        stubs: {
          RouterLink: true
        }
      }
    })
    await flushPromises()

    expect(getDetail).toHaveBeenCalledWith(
      3,
      { range: '7d' },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })
})
