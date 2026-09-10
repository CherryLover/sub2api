import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountStatusIndicator from '../AccountStatusIndicator.vue'
import type { Account, AccountDiagnosis, AccountSchedulingBlock } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${Object.values(params).join(',')}` : key
    })
  }
})

vi.mock('@/utils/format', async () => {
  const actual = await vi.importActual<typeof import('@/utils/format')>('@/utils/format')
  return {
    ...actual,
    formatCountdown: () => '1h',
    formatDateTimeToMinute: (value: string) => `min:${value}`
  }
})

const FUTURE = '2099-09-15T07:10:53Z'
const PAST = '2020-01-01T00:00:00Z'

function makeAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 6,
    name: 'account',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
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
  }
}

function makeDiagnosis(blocks: AccountSchedulingBlock[], schedulable = blocks.length === 0): AccountDiagnosis {
  return {
    scheduling: { schedulable, blocks },
    recent_errors: null
  }
}

function mountIndicator(diagnosis: AccountDiagnosis | null, account: Account = makeAccount()) {
  return mount(AccountStatusIndicator, {
    props: { account, diagnosis },
    global: {
      stubs: { Icon: true, teleport: true }
    }
  })
}

describe('AccountStatusIndicator scheduling diagnosis badge', () => {
  it('lists every in-process block on its own line', () => {
    const wrapper = mountIndicator(
      makeDiagnosis([
        { source: 'runtime_block', until: FUTURE },
        { source: 'model_runtime_block', model: 'gpt-6-astra', until: FUTURE },
        { source: 'proxy_quarantine', proxy_id: 3, until: FUTURE },
        { source: 'quota_auto_pause', window: '7d', threshold: 0.9, utilization: 0.93 }
      ])
    )

    const badge = wrapper.find('[data-test="scheduling-block-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('admin.accounts.status.unschedulable')
    expect(badge.find('.bg-orange-100').exists()).toBe(true)

    const lines = wrapper.find('[data-test="scheduling-block-lines"]').findAll('div').map(line => line.text())
    expect(lines).toEqual([
      `admin.accounts.schedulingBlock.runtimeBlockedUntil|min:${FUTURE}`,
      `admin.accounts.schedulingBlock.modelBlockedUntil|gpt-6-astra,min:${FUTURE}`,
      `admin.accounts.schedulingBlock.proxyQuarantinedUntil|3,min:${FUTURE}`,
      'admin.accounts.schedulingBlock.quotaAutoPaused|7d,93,90'
    ])
  })

  it('does not duplicate reasons already shown by the DB-derived badges', () => {
    const wrapper = mountIndicator(
      makeDiagnosis(
        [
          { source: 'status' },
          { source: 'manual_unschedulable' },
          { source: 'rate_limited', until: FUTURE },
          { source: 'temp_unschedulable', until: FUTURE },
          { source: 'overloaded', until: FUTURE },
          { source: 'quota_exceeded' },
          { source: 'expired' }
        ],
        false
      )
    )

    expect(wrapper.find('[data-test="scheduling-block-badge"]').exists()).toBe(false)
  })

  it('shows only the hidden blocks when mixed with DB-derived ones', () => {
    const wrapper = mountIndicator(
      makeDiagnosis([{ source: 'rate_limited', until: FUTURE }, { source: 'runtime_block', until: FUTURE }], false)
    )

    const lines = wrapper.find('[data-test="scheduling-block-lines"]').findAll('div')
    expect(lines).toHaveLength(1)
    expect(lines[0].text()).toContain('runtimeBlockedUntil')
  })

  it('drops blocks whose until is already in the past', () => {
    const allExpired = mountIndicator(
      makeDiagnosis([{ source: 'runtime_block', until: PAST }, { source: 'proxy_quarantine', proxy_id: 1, until: PAST }], false)
    )
    expect(allExpired.find('[data-test="scheduling-block-badge"]').exists()).toBe(false)

    const mixed = mountIndicator(
      makeDiagnosis([{ source: 'runtime_block', until: PAST }, { source: 'model_runtime_block', model: 'm', until: FUTURE }], false)
    )
    const lines = mixed.find('[data-test="scheduling-block-lines"]').findAll('div')
    expect(lines).toHaveLength(1)
    expect(lines[0].text()).toContain('modelBlockedUntil|m,')
  })

  it('uses a softer "partially blocked" badge when the account is still schedulable', () => {
    const wrapper = mountIndicator(
      makeDiagnosis([{ source: 'model_runtime_block', model: 'gpt-6-astra', until: FUTURE }], true)
    )

    const badge = wrapper.find('[data-test="scheduling-block-badge"]')
    expect(badge.text()).toContain('admin.accounts.status.partiallyBlocked')
    expect(badge.find('.bg-amber-100').exists()).toBe(true)
  })

  it('handles blocks without optional fields', () => {
    const wrapper = mountIndicator(
      makeDiagnosis([{ source: 'runtime_block' }, { source: 'quota_auto_pause' }], false)
    )

    const lines = wrapper.find('[data-test="scheduling-block-lines"]').findAll('div').map(line => line.text())
    expect(lines).toEqual([
      'admin.accounts.schedulingBlock.runtimeBlocked',
      'admin.accounts.schedulingBlock.quotaAutoPausedGeneric'
    ])
  })

  it('renders nothing extra without a diagnosis', () => {
    const wrapper = mountIndicator(null)
    expect(wrapper.find('[data-test="scheduling-block-badge"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('admin.accounts.status.active')
  })
})
