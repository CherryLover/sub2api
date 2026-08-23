/**
 * AccountStatusIndicator —— 「到期即失效」状态的时间响应性回归测试
 *
 * 这些状态（429 限流 / 529 超载 / 临时不可调度 / 模型级限流）在后端是惰性判定的：
 * reset 时刻到了，DB 里那一行一个字节都不会变，列表 ETag 也不会变，自动刷新拿到
 * 304、增量合并保留旧对象引用 —— 于是前端 computed 永远不重算，徽标渲染之后就冻住。
 *
 * 下面每条用例都：挂载 → 推进时间越过 reset 时刻 →（不重新挂载、不改 props）
 * 断言徽标自行消失 / 倒计时自行更新。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import AccountStatusIndicator from '../AccountStatusIndicator.vue'
import { NOW_TICK_INTERVAL_MS, _resetNowTick } from '@/composables/useNowTick'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    // 运行时 i18n 构建不支持编译消息，这里把 key + 参数一起吐出来便于断言
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${Object.values(params).join(',')}` : key
    })
  }
})

// 真实的 formatCountdown 依赖运行时消息编译（测试环境不可用），这里用同样接受
// nowMs 的最小实现替代，好让"倒计时随时间变化"可断言
vi.mock('@/utils/format', async () => {
  const actual = await vi.importActual<typeof import('@/utils/format')>('@/utils/format')
  const countdown = (target: string | Date | null | undefined, nowMs?: number): string | null => {
    if (!target) return null
    const diff = new Date(target).getTime() - (nowMs ?? Date.now())
    if (!(diff > 0)) return null
    return `${Math.floor(diff / 60000)}m`
  }
  return {
    ...actual,
    formatCountdown: countdown,
    formatCountdownWithSuffix: (target: string | Date | null | undefined, nowMs?: number) => {
      const value = countdown(target, nowMs)
      return value ? `${value} to lift` : null
    }
  }
})

const T0 = Date.UTC(2026, 7, 23, 10, 0, 0)
const at = (offsetMs: number) => new Date(T0 + offsetMs).toISOString()

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'antigravity',
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
    ...overrides,
  } as Account
}

function mountIndicator(account: Account) {
  return mount(AccountStatusIndicator, {
    props: { account },
    global: { stubs: { Icon: true } }
  })
}

/** 推进时间并等待 Vue 刷新（fake timers 必须配合 nextTick，否则 DOM 不会更新） */
async function advance(ms: number): Promise<void> {
  vi.advanceTimersByTime(ms)
  await nextTick()
}

describe('AccountStatusIndicator 状态到期自动消失', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(T0)
    _resetNowTick()
  })

  afterEach(() => {
    _resetNowTick()
    vi.useRealTimers()
  })

  it('429 限流徽标在 rate_limit_reset_at 到期后自行消失（核心回归）', async () => {
    const wrapper = mountIndicator(
      makeAccount({
        rate_limited_at: at(-60_000),
        rate_limit_reset_at: at(1_000)
      })
    )

    expect(wrapper.text()).toContain('admin.accounts.status.rateLimited')
    expect(wrapper.text()).toContain('429')

    // 不重新挂载、不改 props，只是时间过去了
    await advance(NOW_TICK_INTERVAL_MS)

    expect(wrapper.text()).not.toContain('admin.accounts.status.rateLimited')
    expect(wrapper.text()).not.toContain('429')
    expect(wrapper.text()).toContain('admin.accounts.status.active')

    wrapper.unmount()
  })

  it('529 超载徽标在 overload_until 到期后自行消失', async () => {
    const wrapper = mountIndicator(makeAccount({ overload_until: at(1_000) }))

    expect(wrapper.text()).toContain('admin.accounts.status.overloaded')
    expect(wrapper.text()).toContain('529')

    await advance(NOW_TICK_INTERVAL_MS)

    expect(wrapper.text()).not.toContain('admin.accounts.status.overloaded')
    expect(wrapper.text()).not.toContain('529')
    expect(wrapper.text()).toContain('admin.accounts.status.active')

    wrapper.unmount()
  })

  it('临时不可调度徽标在 temp_unschedulable_until 到期后自行消失', async () => {
    const wrapper = mountIndicator(
      makeAccount({
        temp_unschedulable_until: at(1_000),
        temp_unschedulable_reason: 'upstream 5xx'
      })
    )

    expect(wrapper.text()).toContain('admin.accounts.status.tempUnschedulable')

    await advance(NOW_TICK_INTERVAL_MS)

    expect(wrapper.text()).not.toContain('admin.accounts.status.tempUnschedulable')
    expect(wrapper.text()).toContain('admin.accounts.status.active')

    wrapper.unmount()
  })

  it('模型级限流徽标在各自 reset_at 到期后逐个消失', async () => {
    const wrapper = mountIndicator(
      makeAccount({
        extra: {
          model_rate_limits: {
            'claude-sonnet-4-5': {
              rate_limited_at: at(-60_000),
              rate_limit_reset_at: at(1_000)
            },
            'claude-opus-5': {
              rate_limited_at: at(-60_000),
              rate_limit_reset_at: at(30 * 60_000)
            }
          }
        }
      })
    )

    expect(wrapper.text()).toContain('CSon45')
    expect(wrapper.text()).toContain('COpus5')

    await advance(NOW_TICK_INTERVAL_MS)

    // 先到期的消失，未到期的保留
    expect(wrapper.text()).not.toContain('CSon45')
    expect(wrapper.text()).toContain('COpus5')

    await advance(30 * 60_000)

    expect(wrapper.text()).not.toContain('COpus5')

    wrapper.unmount()
  })

  it('AICredits 积分耗尽徽标在到期后自行消失', async () => {
    const wrapper = mountIndicator(
      makeAccount({
        extra: {
          allow_overages: true,
          model_rate_limits: {
            AICredits: {
              rate_limited_at: at(-60_000),
              rate_limit_reset_at: at(1_000)
            }
          }
        }
      })
    )

    expect(wrapper.text()).toContain('admin.accounts.status.creditsExhausted')

    await advance(NOW_TICK_INTERVAL_MS)

    expect(wrapper.text()).not.toContain('admin.accounts.status.creditsExhausted')

    wrapper.unmount()
  })

  it('429 倒计时文字随时间递减', async () => {
    const wrapper = mountIndicator(makeAccount({ rate_limit_reset_at: at(10 * 60_000) }))

    expect(wrapper.text()).toContain('admin.accounts.status.rateLimitedAutoResume|10m')

    await advance(4 * 60_000)
    expect(wrapper.text()).toContain('admin.accounts.status.rateLimitedAutoResume|6m')

    await advance(5 * 60_000)
    expect(wrapper.text()).toContain('admin.accounts.status.rateLimitedAutoResume|1m')

    wrapper.unmount()
  })

  it('529 倒计时文字随时间递减', async () => {
    const wrapper = mountIndicator(makeAccount({ overload_until: at(10 * 60_000) }))

    expect(wrapper.text()).toContain('10m to lift')

    await advance(7 * 60_000)
    expect(wrapper.text()).toContain('3m to lift')

    wrapper.unmount()
  })

  it('模型级限流倒计时文字随时间递减', async () => {
    const wrapper = mountIndicator(
      makeAccount({
        extra: {
          model_rate_limits: {
            'claude-sonnet-4-5': {
              rate_limited_at: at(-60_000),
              rate_limit_reset_at: at(10 * 60_000)
            }
          }
        }
      })
    )

    expect(wrapper.text()).toContain('10m')

    await advance(6 * 60_000)
    expect(wrapper.text()).toContain('4m')

    wrapper.unmount()
  })

  it('已经过期的时间戳在挂载时就不显示徽标', () => {
    const wrapper = mountIndicator(
      makeAccount({
        rate_limit_reset_at: at(-1_000),
        overload_until: at(-1_000),
        temp_unschedulable_until: at(-1_000)
      })
    )

    expect(wrapper.text()).not.toContain('admin.accounts.status.rateLimited')
    expect(wrapper.text()).not.toContain('admin.accounts.status.overloaded')
    expect(wrapper.text()).not.toContain('admin.accounts.status.tempUnschedulable')
    expect(wrapper.text()).toContain('admin.accounts.status.active')

    wrapper.unmount()
  })

  it('非法时间戳不会被当成限流中', () => {
    const wrapper = mountIndicator(makeAccount({ rate_limit_reset_at: 'not-a-date' }))

    expect(wrapper.text()).not.toContain('admin.accounts.status.rateLimited')
    expect(wrapper.text()).toContain('admin.accounts.status.active')

    wrapper.unmount()
  })

  it('账号列表里的多行共用同一个心跳定时器', async () => {
    const rows = Array.from({ length: 25 }, (_, index) =>
      mountIndicator(makeAccount({ id: index + 1, rate_limit_reset_at: at(1_000) }))
    )

    // 25 行 → 依然只有 1 个定时器
    expect(vi.getTimerCount()).toBe(1)

    await advance(NOW_TICK_INTERVAL_MS)

    for (const row of rows) {
      expect(row.text()).not.toContain('429')
    }

    rows.forEach(row => row.unmount())
    expect(vi.getTimerCount()).toBe(0)
  })
})
