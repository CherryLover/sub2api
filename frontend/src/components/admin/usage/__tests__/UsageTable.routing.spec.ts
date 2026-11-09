const ipGeoMocks = vi.hoisted(() => ({
  getEntry: vi.fn(() => ({ status: 'idle' as const })),
  fetchOne: vi.fn(),
  fetchBatch: vi.fn(),
}))

vi.mock('@/utils/ipGeoLookup', () => ipGeoMocks)
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'

import zh from '@/i18n/locales/zh'
import UsageTable from '../UsageTable.vue'
import type { AdminUsageLog, RoutingInfo } from '@/types'
import { loadBalanceInfoFixture, switchedInfoFixture } from './routingFixtures'

const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })

function log(id: number, routing_info: RoutingInfo | null): AdminUsageLog {
  return {
    id,
    user_id: 1,
    api_key_id: 5,
    account_id: 1,
    request_id: `req-${id}`,
    model: 'gpt-6.1-sol',
    api_key: { id: 5, name: 'jiangjiwei' },
    group: { id: 3, name: 'Composite' },
    account: { id: 1, name: 'g1.jacobs' },
    duration_ms: 2650,
    first_token_ms: 1810,
    created_at: '2026-11-09T02:12:39Z',
    routing_info,
  } as unknown as AdminUsageLog
}

function mountTable(data: AdminUsageLog[]) {
  return mount(UsageTable, {
    props: { data, columns: [{ key: 'user', label: 'user' }, { key: 'routing', label: '选号' }] },
    global: { plugins: [i18n] },
  })
}

describe('UsageTable routing column', () => {
  it('shows a chip label for every routing method and a fallback for old rows', () => {
    const wrapper = mountTable([
      log(1, loadBalanceInfoFixture),
      log(2, { method: 'sticky', candidate_count: 3 }),
      log(3, switchedInfoFixture),
      log(4, { method: 'load_balance', candidate_count: 1, queue_wait_ms: 2100 }),
      log(5, { method: 'single', candidate_count: 1 }),
      log(6, null),
    ])
    expect(wrapper.findAll('[data-test="routing-chip"]').map((b) => b.text().replace(/\s*[▾▴]$/, ''))).toEqual([
      '负载均衡 · 3 选 1',
      '沿用上次账号',
      '切换过 1 次账号',
      '负载均衡 · 排队 2.1s',
      '只有 1 个候选',
    ])
    expect(wrapper.get('[data-test="routing-chip-empty"]').text()).toBe('无选号记录')
  })

  it('toggles the inline routing panel when the chip is clicked', async () => {
    const wrapper = mountTable([log(1, loadBalanceInfoFixture), log(2, switchedInfoFixture)])
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(0)

    await wrapper.findAll('[data-test="routing-chip"]')[1].trigger('click')
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-test="routing-info-attempt"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-test="routing-chip"]')[1].attributes('aria-expanded')).toBe('true')

    await wrapper.findAll('[data-test="routing-chip"]')[0].trigger('click')
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="routing-candidates-list"]').exists()).toBe(true)

    await wrapper.findAll('[data-test="routing-chip"]')[1].trigger('click')
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="routing-info-attempts"]').exists()).toBe(false)
  })
})
