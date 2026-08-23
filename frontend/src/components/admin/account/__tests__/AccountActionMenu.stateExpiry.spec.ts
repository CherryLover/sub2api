/**
 * AccountActionMenu —— 限流/超载/临时不可调度到期后「恢复状态」入口自行隐藏
 *
 * 与 AccountStatusIndicator 同一个根因：这些状态靠 `now > *_at` 惰性判定，
 * new Date() 不是响应式依赖，菜单打开期间跨过 reset 时刻不会重算。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import AccountActionMenu from '../AccountActionMenu.vue'
import { NOW_TICK_INTERVAL_MS, _resetNowTick } from '@/composables/useNowTick'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const T0 = Date.UTC(2026, 7, 23, 10, 0, 0)
const at = (offsetMs: number) => new Date(T0 + offsetMs).toISOString()

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'test-account',
    platform: 'openai',
    type: 'apikey',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
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
    ...overrides,
  } as Account
}

const position = { top: 100, left: 100 }
const bodyText = () => document.body.textContent ?? ''

describe('AccountActionMenu 状态到期', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(T0)
    _resetNowTick()
  })

  afterEach(() => {
    _resetNowTick()
    vi.useRealTimers()
  })

  it.each([
    ['rate_limit_reset_at', { rate_limit_reset_at: at(1_000) }],
    ['overload_until', { overload_until: at(1_000) }],
    ['temp_unschedulable_until', { temp_unschedulable_until: at(1_000) }],
    [
      'model_rate_limits',
      {
        extra: {
          model_rate_limits: {
            'claude-sonnet-4-5': { rate_limit_reset_at: at(1_000) },
          },
        },
      },
    ],
  ])('%s 到期后「恢复状态」入口自行消失', async (_label, overrides) => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: makeAccount(overrides as Partial<Account>), position },
      attachTo: document.body,
    })

    expect(bodyText()).toContain('admin.accounts.recoverState')

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS)
    await nextTick()

    expect(bodyText()).not.toContain('admin.accounts.recoverState')

    wrapper.unmount()
  })

  it('status=error 的账号不受心跳影响，「恢复状态」始终可见', async () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account: makeAccount({ status: 'error' }), position },
      attachTo: document.body,
    })

    expect(bodyText()).toContain('admin.accounts.recoverState')

    vi.advanceTimersByTime(NOW_TICK_INTERVAL_MS * 10)
    await nextTick()

    expect(bodyText()).toContain('admin.accounts.recoverState')

    wrapper.unmount()
  })
})
