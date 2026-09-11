import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { Account } from '@/types'

import ModelAvailabilityView from '../ModelAvailabilityView.vue'

const { listAccounts, getBatchDiagnostics, getAvailableModels } = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  getBatchDiagnostics: vi.fn(),
  getAvailableModels: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      getBatchDiagnostics,
      getAvailableModels
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${Object.values(params).join(' ')}` : key
    })
  }
})

const FUTURE = new Date(Date.now() + 3 * 24 * 3600 * 1000).toISOString()
const PAST = new Date(Date.now() - 3 * 24 * 3600 * 1000).toISOString()

function makeAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 1,
    name: 'acct',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 0,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  } as Account
}

function mountView() {
  return mount(ModelAvailabilityView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' }
      }
    }
  })
}

const modelsOf = (...ids: string[]) => ids.map((id) => ({ id, type: 'model', display_name: id, created_at: '' }))

beforeEach(() => {
  vi.clearAllMocks()
  listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 200, pages: 1 })
  getBatchDiagnostics.mockResolvedValue({ window_minutes: 15, generated_at: '', diagnostics: {} })
  getAvailableModels.mockResolvedValue([])
})

describe('ModelAvailabilityView', () => {
  it('renders the account x model matrix and the summary counters', async () => {
    const limited = makeAccount({
      id: 1,
      name: 'spark-account',
      extra: {
        model_rate_limits: {
          'gpt-5.3-codex-spark': {
            rate_limited_at: PAST,
            rate_limit_reset_at: FUTURE,
            reason: 'openai_dedicated_quota_pool_rate_limited'
          }
        }
      }
    })
    const blocked = makeAccount({ id: 2, name: 'paused-account', schedulable: false })
    listAccounts.mockResolvedValue({ items: [limited, blocked], total: 2, page: 1, page_size: 200, pages: 1 })
    getAvailableModels.mockImplementation(async (id: number) =>
      id === 1 ? modelsOf('gpt-5.3-codex', 'gpt-5.3-codex-spark') : modelsOf('gpt-5.3-codex')
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="summary-accounts"]').text()).toBe('2')
    expect(wrapper.find('[data-test="summary-models"]').text()).toBe('2')
    expect(wrapper.find('[data-test="summary-blocked-accounts"]').text()).toBe('1')
    expect(wrapper.find('[data-test="summary-limited-cells"]').text()).toBe('1')

    expect(wrapper.find('[data-test="matrix-row-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="matrix-row-2"] [data-test="account-blocked-badge"]').exists()).toBe(true)

    const cell = wrapper.find('[data-test="cell-1-gpt-5.3-codex-spark"]')
    expect(cell.attributes('data-state')).toBe('limited')
    expect(cell.text()).toContain('admin.accounts.modelAvailability.matrix.limitedUntil')
    expect(cell.attributes('title')).toContain(
      'admin.accounts.modelAvailability.reasons.openaiDedicatedQuotaPool'
    )
    expect(wrapper.find('[data-test="last-updated"]').exists()).toBe(true)
  })

  it('does not count an already expired rate limit', async () => {
    listAccounts.mockResolvedValue({
      items: [
        makeAccount({
          id: 3,
          name: 'stale',
          extra: {
            model_rate_limits: { 'model-x': { rate_limited_at: PAST, rate_limit_reset_at: PAST } }
          }
        })
      ],
      total: 1,
      page: 1,
      page_size: 200,
      pages: 1
    })
    getAvailableModels.mockResolvedValue(modelsOf('model-x'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="summary-limited-cells"]').text()).toBe('0')
    expect(wrapper.find('[data-test="cell-3-model-x"]').attributes('data-state')).toBe('ok')
  })

  it('falls back gracefully when diagnostics fail', async () => {
    listAccounts.mockResolvedValue({
      items: [makeAccount({ id: 4, name: 'acct-4' })],
      total: 1,
      page: 1,
      page_size: 200,
      pages: 1
    })
    getAvailableModels.mockResolvedValue(modelsOf('model-x'))
    getBatchDiagnostics.mockRejectedValue(new Error('boom'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="degraded-notice"]').text()).toContain(
      'admin.accounts.modelAvailability.diagnosticsUnavailable'
    )
    // 诊断挂了也要把矩阵画出来
    expect(wrapper.find('[data-test="cell-4-model-x"]').attributes('data-state')).toBe('ok')
    expect(wrapper.find('[data-test="error-state"]').exists()).toBe(false)
  })

  it('reports accounts whose model list could not be fetched', async () => {
    listAccounts.mockResolvedValue({
      items: [makeAccount({ id: 5, name: 'ok' }), makeAccount({ id: 6, name: 'broken' })],
      total: 2,
      page: 1,
      page_size: 200,
      pages: 1
    })
    getAvailableModels.mockImplementation(async (id: number) => {
      if (id === 6) throw new Error('nope')
      return modelsOf('model-x')
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="degraded-notice"]').text()).toContain(
      'admin.accounts.modelAvailability.modelListPartiallyUnavailable 1'
    )
    expect(wrapper.find('[data-test="cell-6-model-x"]').attributes('data-state')).toBe('unknown')
  })

  it('shows the empty state when there is no account at all', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="empty-state"]').text()).toContain(
      'admin.accounts.modelAvailability.empty.title'
    )
    expect(wrapper.find('[data-test="matrix-scroll"]').exists()).toBe(false)
    // 没有账号时不该再去打诊断接口
    expect(getBatchDiagnostics).not.toHaveBeenCalled()
  })

  it('shows an error state with a retry button when the account list fails', async () => {
    listAccounts.mockRejectedValueOnce(new Error('network down'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="error-state"]').text()).toContain('network down')

    listAccounts.mockResolvedValueOnce({
      items: [makeAccount({ id: 7, name: 'back' })],
      total: 1,
      page: 1,
      page_size: 200,
      pages: 1
    })
    getAvailableModels.mockResolvedValue(modelsOf('model-x'))

    await wrapper.find('[data-test="retry-button"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="error-state"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="matrix-row-7"]').exists()).toBe(true)
  })

  it('refreshes on demand and can narrow the view to problems only', async () => {
    const limited = makeAccount({
      id: 8,
      name: 'limited',
      extra: { model_rate_limits: { 'model-y': { rate_limited_at: PAST, rate_limit_reset_at: FUTURE } } }
    })
    listAccounts.mockResolvedValue({
      items: [limited, makeAccount({ id: 9, name: 'healthy' })],
      total: 2,
      page: 1,
      page_size: 200,
      pages: 1
    })
    getAvailableModels.mockResolvedValue(modelsOf('model-x', 'model-y'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-test="account-cell"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-test="model-header"]')).toHaveLength(2)

    await wrapper.find('[data-test="only-issues"]').setValue(true)
    await flushPromises()

    expect(wrapper.findAll('[data-test="account-cell"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-test="model-header"]')).toHaveLength(1)
    // 汇总条仍然描述整体情况
    expect(wrapper.find('[data-test="summary-accounts"]').text()).toBe('2')

    expect(listAccounts).toHaveBeenCalledTimes(1)
    await wrapper.find('[data-test="refresh-button"]').trigger('click')
    await flushPromises()
    expect(listAccounts).toHaveBeenCalledTimes(2)
  })

  it('filters by platform', async () => {
    listAccounts.mockResolvedValue({
      items: [
        makeAccount({ id: 10, name: 'openai-one', platform: 'openai' }),
        makeAccount({ id: 11, name: 'grok-one', platform: 'grok' })
      ],
      total: 2,
      page: 1,
      page_size: 200,
      pages: 1
    })
    getAvailableModels.mockResolvedValue(modelsOf('model-x'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-test="account-cell"]')).toHaveLength(2)

    await wrapper.find('[data-test="platform-filter"]').setValue('grok')
    await flushPromises()

    expect(wrapper.findAll('[data-test="account-cell"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="matrix-row-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="summary-accounts"]').text()).toBe('1')
  })
})
