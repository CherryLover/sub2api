import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import type { Account, AccountDiagnosis } from '@/types'

import AvailabilityMatrixTable from '../AvailabilityMatrixTable.vue'
import { buildModelAvailabilityMatrix, type ModelAvailabilityInput } from '../modelAvailability'

// t 回显 key + 参数值，这样断言既能验证用了哪条文案，也能看到插进去的时间
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

const NOW = Date.parse('2026-09-11T10:00:00Z')
const FUTURE = '2026-09-15T07:04:37Z'
const PAST = '2026-09-01T07:04:37Z'

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

function mountMatrix(inputs: ModelAvailabilityInput[]) {
  const matrix = buildModelAvailabilityMatrix(inputs, NOW)
  return mount(AvailabilityMatrixTable, {
    props: { columns: matrix.columns, rows: matrix.rows }
  })
}

const diagnosis = (blocks: AccountDiagnosis['scheduling']['blocks'], schedulable = true): AccountDiagnosis => ({
  scheduling: { schedulable, blocks },
  recent_errors: null
})

describe('AvailabilityMatrixTable', () => {
  it('renders an account row and a model column per entry', () => {
    const wrapper = mountMatrix([
      { account: makeAccount({ id: 1, name: 'alpha' }), models: ['gpt-5.3-codex', 'gpt-5.3-codex-spark'] },
      { account: makeAccount({ id: 2, name: 'bravo' }), models: ['gpt-5.3-codex'] }
    ])

    expect(wrapper.findAll('[data-test="model-header"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="matrix-row-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="matrix-row-2"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="cell-1-gpt-5.3-codex"]').attributes('data-state')).toBe('ok')
    expect(wrapper.find('[data-test="cell-2-gpt-5.3-codex-spark"]').attributes('data-state')).toBe('unsupported')
  })

  it('shows the reset time and the translated reason on a model-level rate limit', () => {
    const wrapper = mountMatrix([
      {
        account: makeAccount({
          id: 5,
          name: 'spark',
          extra: {
            model_rate_limits: {
              'gpt-5.3-codex-spark': {
                rate_limited_at: PAST,
                rate_limit_reset_at: FUTURE,
                reason: 'openai_dedicated_quota_pool_rate_limited'
              }
            }
          }
        }),
        models: ['gpt-5.3-codex-spark']
      }
    ])

    const cell = wrapper.find('[data-test="cell-5-gpt-5.3-codex-spark"]')
    expect(cell.attributes('data-state')).toBe('limited')
    expect(cell.text()).toContain('admin.accounts.modelAvailability.matrix.limitedUntil')
    // 「限流至 09-15 15:04」：月-日 时:分，随浏览器时区渲染
    expect(cell.text()).toMatch(/\d{2}[-/]\d{2}[ ,]+\d{2}:\d{2}/)
    // 悬停能看到人话原因
    expect(cell.attributes('title')).toContain(
      'admin.accounts.modelAvailability.reasons.openaiDedicatedQuotaPool'
    )
  })

  it('shows unknown reasons verbatim in the tooltip', () => {
    const wrapper = mountMatrix([
      {
        account: makeAccount({
          id: 6,
          name: 'weird',
          extra: {
            model_rate_limits: {
              'model-x': {
                rate_limited_at: PAST,
                rate_limit_reset_at: FUTURE,
                reason: 'brand_new_backend_reason'
              }
            }
          }
        }),
        models: ['model-x']
      }
    ])

    expect(wrapper.find('[data-test="cell-6-model-x"]').attributes('title')).toContain(
      'brand_new_backend_reason'
    )
  })

  it('does not render an expired rate limit as limited', () => {
    const wrapper = mountMatrix([
      {
        account: makeAccount({
          id: 7,
          name: 'stale',
          extra: {
            model_rate_limits: {
              'model-x': { rate_limited_at: PAST, rate_limit_reset_at: PAST, reason: 'whatever' }
            }
          }
        }),
        models: ['model-x']
      }
    ])

    const cell = wrapper.find('[data-test="cell-7-model-x"]')
    expect(cell.attributes('data-state')).toBe('ok')
    expect(cell.text()).not.toContain('limitedUntil')
    expect(cell.text()).toContain('✓')
  })

  it('marks the whole row when the account itself is unschedulable', () => {
    const wrapper = mountMatrix([
      {
        account: makeAccount({ id: 8, name: 'down', schedulable: false }),
        diagnosis: diagnosis([{ source: 'manual_unschedulable' }, { source: 'runtime_block', until: FUTURE }], false),
        models: ['model-x']
      },
      { account: makeAccount({ id: 9, name: 'fine' }), models: ['model-x'] }
    ])

    const blockedRow = wrapper.find('[data-test="matrix-row-8"]')
    expect(blockedRow.find('[data-test="account-blocked-badge"]').exists()).toBe(true)
    expect(blockedRow.classes().join(' ')).toContain('bg-gray-100/80')

    const hint = blockedRow.find('[data-test="account-blocked-reason"]').text()
    expect(hint).toContain('admin.accounts.modelAvailability.matrix.accountBlockedHint')
    expect(hint).toContain('admin.accounts.modelAvailability.accountBlocks.manualUnschedulable')
    expect(hint).toContain('admin.accounts.modelAvailability.accountBlocks.runtimeBlock')

    expect(wrapper.find('[data-test="matrix-row-9"] [data-test="account-blocked-badge"]').exists()).toBe(false)
  })

  it('keeps the account column pinned to the left and never stacks names vertically', () => {
    const wrapper = mountMatrix([
      {
        account: makeAccount({ id: 10, name: 'a-very-long-account-name-that-should-be-truncated-not-wrapped' }),
        models: ['model-x']
      }
    ])

    const header = wrapper.find('[data-test="account-header"]')
    expect(header.classes()).toContain('sticky')
    expect(header.classes()).toContain('left-0')

    const cell = wrapper.find('[data-test="account-cell"]')
    expect(cell.classes()).toContain('sticky')
    expect(cell.classes()).toContain('left-0')

    const name = wrapper.find('[data-test="account-name"]')
    expect(name.classes()).toContain('whitespace-nowrap')
    expect(name.classes()).toContain('truncate')
    // 超长名字靠悬停看全名，不允许竖排
    expect(name.attributes('title')).toBe('a-very-long-account-name-that-should-be-truncated-not-wrapped')

    const modelHeader = wrapper.find('[data-test="model-header"] span')
    expect(modelHeader.classes()).toContain('whitespace-nowrap')
    expect(modelHeader.classes()).toContain('truncate')
  })

  it('labels pseudo scopes and shows how many accounts are limited per model', () => {
    const limited = (id: number, name: string) =>
      makeAccount({
        id,
        name,
        platform: 'antigravity',
        extra: { model_rate_limits: { AICredits: { rate_limited_at: PAST, rate_limit_reset_at: FUTURE } } }
      })

    const wrapper = mountMatrix([
      { account: limited(11, 'a'), models: ['gemini-3-pro'] },
      { account: limited(12, 'b'), models: ['gemini-3-pro'] }
    ])

    const firstHeader = wrapper.findAll('[data-test="model-header"]')[0]
    expect(firstHeader.text()).toContain('admin.accounts.modelAvailability.pseudoModels.aiCredits')
    expect(firstHeader.text()).toContain('admin.accounts.modelAvailability.matrix.limitedAccounts 2')
    expect(firstHeader.attributes('title')).toContain('AICredits')
  })

  it('shows unknown cells when the account model list is unavailable', () => {
    const wrapper = mountMatrix([
      { account: makeAccount({ id: 13, name: 'known' }), models: ['model-x'] },
      { account: makeAccount({ id: 14, name: 'unknown' }) }
    ])

    const cell = wrapper.find('[data-test="cell-14-model-x"]')
    expect(cell.attributes('data-state')).toBe('unknown')
    expect(cell.text()).toBe('?')
    expect(cell.attributes('title')).toBe('admin.accounts.modelAvailability.matrix.unknownHint')
    expect(wrapper.find('[data-test="matrix-row-14"] [data-test="account-models-unavailable"]').exists()).toBe(
      true
    )
  })
})
