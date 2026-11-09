import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TopologyView from '../TopologyView.vue'
import type { TopologySnapshot } from '@/api/admin/topology'

const { getSnapshot, openStream, closeStream } = vi.hoisted(() => ({
  getSnapshot: vi.fn(),
  openStream: vi.fn(),
  closeStream: vi.fn()
}))

vi.mock('@/api/admin/topology', () => ({
  getTopologySnapshot: getSnapshot,
  openTopologyEventStream: openStream
}))

vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual('vue-i18n')),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (!params) return key
      return `${key} ${Object.entries(params).map(([name, value]) => `${name}=${String(value)}`).join(' ')}`
    }
  })
}))

const snapshot: TopologySnapshot = {
  generated_at: '2026-11-09T06:00:00Z',
  totals: {
    in_flight: 1,
    queued: 0,
    requests_5m: 9,
    failed_5m: 1,
    retries_same_5m: 1,
    retries_switch_5m: 2,
    failed_after_retry_5m: 0,
    accounts_available: 1,
    accounts_total: 1
  },
  gate_blocks_5m: { model_not_supported: 2 },
  keys: [
    {
      id: 7,
      name: 'Production key',
      group_id: 3,
      group_name: 'Composite',
      in_flight: 1,
      queued: 0,
      requests_5m: 9,
      failed_5m: 1
    },
    {
      id: 8,
      name: 'Idle key',
      group_id: 3,
      group_name: 'Composite',
      in_flight: 0,
      queued: 0,
      requests_5m: 0,
      failed_5m: 0
    }
  ],
  groups: [
    {
      id: 3,
      name: 'Composite',
      platform: 'openai',
      strategy: 'load_balance',
      in_flight: 1,
      requests_5m: 9,
      failed_5m: 1,
      account_ids: [11]
    }
  ],
  accounts: [
    {
      id: 11,
      name: 'Primary account',
      platform: 'OpenAI',
      status: 'ok',
      status_text: '',
      recover_at: null,
      concurrency_current: 1,
      concurrency_max: 10,
      quota_5h: { used_percent: 12, reset_at: '2026-11-09T09:00:00Z' },
      quota_7d: null,
      billing: 'subscription'
    }
  ]
}

function render() {
  return mount(TopologyView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        RouterLink: { props: ['to'], template: '<a><slot /></a>' }
      }
    }
  })
}

beforeEach(() => {
  getSnapshot.mockReset().mockResolvedValue(snapshot)
  closeStream.mockReset()
  openStream.mockReset().mockReturnValue({ close: closeStream })
})

describe('TopologyView', () => {
  it('loads the snapshot and renders visible key, platform, and account cards', async () => {
    const wrapper = render()
    await flushPromises()

    expect(getSnapshot).toHaveBeenCalledTimes(1)
    expect(openStream).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="topology-loading"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="topology-totals"]').text()).toContain('1')

    const keys = wrapper.findAll('[data-testid="topology-key"]')
    expect(keys).toHaveLength(1)
    expect(keys[0]!.text()).toContain('Production key')
    expect(keys[0]!.text()).toContain('Composite')
    expect(wrapper.get('[data-testid="topology-folded"]').text()).toContain('n=1')

    expect(wrapper.get('[data-testid="topology-platform"]').text()).toContain('admin.topology.gates.auth.title')
    expect(wrapper.get('[data-testid="topology-top-block"]').text()).toContain('2')

    const accounts = wrapper.findAll('[data-testid="topology-account"]')
    expect(accounts).toHaveLength(1)
    expect(accounts[0]!.text()).toContain('Primary account')
    expect(accounts[0]!.get('[data-testid="topology-quota-5h"]').text()).toContain('percent=12')

    wrapper.unmount()
    expect(closeStream).toHaveBeenCalledTimes(1)
  })
})
