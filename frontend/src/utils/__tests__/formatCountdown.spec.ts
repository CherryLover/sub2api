/**
 * formatCountdown / formatCountdownWithSuffix 的可注入"当前时间"参数
 *
 * 注意：测试环境用的是 vue-i18n 运行时构建，不支持消息编译，t() 会原样返回 key。
 * 所以这里断言的是"选中了哪个 key / 是否为 null"，这已足以覆盖 nowMs 的分支行为。
 */
import { describe, expect, it, vi, afterEach } from 'vitest'

import { formatCountdown, formatCountdownWithSuffix } from '../format'

const T0 = Date.UTC(2026, 7, 23, 10, 0, 0)
const at = (offsetMs: number) => new Date(T0 + offsetMs).toISOString()

const MINUTES_KEY = 'common.time.countdown.minutes'
const HOURS_KEY = 'common.time.countdown.hoursMinutes'
const DAYS_KEY = 'common.time.countdown.daysHours'
const SUFFIX_KEY = 'common.time.countdown.withSuffix'

describe('formatCountdown 的 nowMs 参数', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('按传入的 nowMs 计算剩余量级', () => {
    expect(formatCountdown(at(5 * 60_000), T0)).toBe(MINUTES_KEY)
    expect(formatCountdown(at(90 * 60_000), T0)).toBe(HOURS_KEY)
    expect(formatCountdown(at(50 * 60 * 60_000), T0)).toBe(DAYS_KEY)
  })

  it('nowMs 越过目标时间后返回 null（徽标据此消失）', () => {
    const target = at(60_000)

    expect(formatCountdown(target, T0)).toBe(MINUTES_KEY)
    expect(formatCountdown(target, T0 + 59_000)).toBe(MINUTES_KEY)
    expect(formatCountdown(target, T0 + 60_000)).toBeNull()
    expect(formatCountdown(target, T0 + 61_000)).toBeNull()
  })

  it('省略 nowMs 时退回 Date.now()', () => {
    vi.useFakeTimers()
    vi.setSystemTime(T0)

    const target = at(5 * 60_000)
    expect(formatCountdown(target)).toBe(MINUTES_KEY)

    vi.setSystemTime(T0 + 10 * 60_000)
    expect(formatCountdown(target)).toBeNull()
  })

  it('空值与非法时间仍然返回 null', () => {
    expect(formatCountdown(null, T0)).toBeNull()
    expect(formatCountdown(undefined, T0)).toBeNull()
    expect(formatCountdown('', T0)).toBeNull()
    expect(formatCountdown('not-a-date', T0)).toBeNull()
  })

  it('formatCountdownWithSuffix 透传 nowMs', () => {
    const target = at(60_000)

    expect(formatCountdownWithSuffix(target, T0)).toBe(SUFFIX_KEY)
    expect(formatCountdownWithSuffix(target, T0 + 60_000)).toBeNull()
  })
})
