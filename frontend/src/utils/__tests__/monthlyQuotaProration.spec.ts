import { describe, expect, it } from 'vitest'
import {
  estimateMonthlyProration,
  getMonthlyEffectiveLimit,
  isMonthlyProrationActive,
  prorateMonthlyLimit,
  resolveFixedMonthlyPeriod,
  roundMonthlyQuota,
} from '../monthlyQuotaProration'

describe('monthly quota proration', () => {
  it('rounds the prorated amount to two decimals', () => {
    const start = new Date('2026-11-01T00:00:00Z')
    const end = new Date('2026-11-30T00:00:00Z')
    const from = new Date('2026-11-10T00:00:00Z')

    expect(prorateMonthlyLimit(100, start, end, from)).toBe(68.97)
    expect(roundMonthlyQuota(10 / 3)).toBe(3.33)
  })

  it('resolves a fixed period across a short month in Shanghai time', () => {
    const now = new Date('2026-02-15T00:00:00+08:00')
    const period = resolveFixedMonthlyPeriod(now, 31, 0, 'Asia/Shanghai')

    expect(period?.start.toISOString()).toBe('2026-01-30T16:00:00.000Z')
    expect(period?.end.toISOString()).toBe('2026-02-27T16:00:00.000Z')
  })

  it('estimates the remaining part of a cross-month cycle', () => {
    const now = new Date('2026-11-26T00:00:00+08:00')
    const estimate = estimateMonthlyProration(100, 1, 0, 'Asia/Shanghai', now)

    expect(estimate?.effectiveLimit).toBe(16.67)
    expect(estimate?.remainingDays).toBe(5)
    expect(estimate?.totalDays).toBe(30)
    expect(estimate?.start.toISOString()).toBe('2026-10-31T16:00:00.000Z')
    expect(estimate?.end.toISOString()).toBe('2026-11-30T16:00:00.000Z')
  })

  it('treats the exact reset instant as the start of a complete new cycle', () => {
    const now = new Date('2026-12-01T00:00:00+08:00')
    const estimate = estimateMonthlyProration(100, 1, 0, 'Asia/Shanghai', now)

    expect(estimate?.effectiveLimit).toBe(100)
    expect(estimate?.remainingDays).toBe(31)
    expect(estimate?.totalDays).toBe(31)
  })

  it('uses the prorated limit only before prorated_until', () => {
    const state = {
      prorate: true,
      proratedLimit: 50,
      proratedUntil: '2026-12-01T00:00:00Z',
    }

    expect(isMonthlyProrationActive(state, new Date('2026-11-30T23:59:59Z'))).toBe(true)
    expect(getMonthlyEffectiveLimit(100, state, new Date('2026-11-30T23:59:59Z'))).toBe(50)
    expect(isMonthlyProrationActive(state, new Date('2026-12-01T00:00:00Z'))).toBe(false)
    expect(getMonthlyEffectiveLimit(100, state, new Date('2026-12-01T00:00:00Z'))).toBe(100)
  })
})
