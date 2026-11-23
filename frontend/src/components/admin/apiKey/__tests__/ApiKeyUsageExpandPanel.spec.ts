import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { ApiKey } from '@/types'
import ApiKeyUsageExpandPanel from '../ApiKeyUsageExpandPanel.vue'

const { getModelStats } = vi.hoisted(() => ({
  getModelStats: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getModelStats,
    },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const mockKey: ApiKey = {
  id: 21,
  user_id: 1,
  key: 'sk-abc...wxyz',
  name: 'team-key',
  group_id: 6,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota_used: 12.5,
  expires_at: null,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  current_concurrency: 0,
  user: { id: 1, email: 'owner@example.com', username: 'owner' },
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
}

describe('ApiKeyUsageExpandPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads today upstream model stats on mount and renders family summary + rows', async () => {
    getModelStats.mockResolvedValueOnce({
      models: [
        {
          model: 'deepseek-flash',
          requests: 120,
          input_tokens: 100000,
          output_tokens: 20000,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          total_tokens: 120000,
          cost: 0.4,
          actual_cost: 0.4,
        },
        {
          model: 'gpt-6.1-sol',
          requests: 40,
          input_tokens: 50000,
          output_tokens: 10000,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          total_tokens: 60000,
          cost: 1.6,
          actual_cost: 1.6,
        },
      ],
      start_date: '2026-11-23',
      end_date: '2026-11-23',
    })

    const wrapper = mount(ApiKeyUsageExpandPanel, {
      props: {
        apiKey: mockKey,
        usageStat: {
          api_key_id: 21,
          today_actual_cost: 2.0,
          total_actual_cost: 12.5,
        },
      },
      global: {
        stubs: {
          Icon: { template: '<span />' },
        },
      },
    })

    await flushPromises()

    expect(getModelStats).toHaveBeenCalledTimes(1)
    const [params] = getModelStats.mock.calls[0]
    expect(params).toMatchObject({
      api_key_id: 21,
      model_source: 'upstream',
    })
    expect(params.start_date).toBe(params.end_date)

    const familyCards = wrapper.findAll('[data-testid="model-family-card"]')
    expect(familyCards.map((c) => c.attributes('data-family'))).toEqual(['openai', 'deepseek'])

    const rows = wrapper.findAll('[data-test="expand-model-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('gpt-6.1-sol')
    expect(rows[1].text()).toContain('deepseek-flash')
  })

  it('switches time window and queries new date range with model_source=upstream', async () => {
    getModelStats.mockResolvedValue({
      models: [],
      start_date: '2026-11-17',
      end_date: '2026-11-23',
    })

    const wrapper = mount(ApiKeyUsageExpandPanel, {
      props: { apiKey: mockKey },
      global: {
        stubs: {
          Icon: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-test="expand-window-7d"]').trigger('click')
    await flushPromises()

    expect(getModelStats).toHaveBeenCalledTimes(2)
    const [secondParams] = getModelStats.mock.calls[1]
    expect(secondParams.model_source).toBe('upstream')
    expect(secondParams.start_date).not.toBe(secondParams.end_date)
    expect(wrapper.find('[data-test="expand-empty"]').exists()).toBe(true)
  })
})
