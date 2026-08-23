/**
 * Count abbreviation for the public Key Usage page.
 *
 * The rule the station owner asked for: keep the digits available, but stop rendering
 * nine-digit token totals inline. Everything at or above a million collapses to three
 * significant digits with an M/B suffix; everything below keeps its exact grouped form.
 */
import { describe, expect, it } from 'vitest'

import { formatCount, formatCountExact, metricTitleOf, metricValueOf } from '../keyUsageFormat'

describe('formatCount', () => {
  it('keeps counts below a million exact', () => {
    expect(formatCount(0)).toBe('0')
    expect(formatCount(1)).toBe('1')
    expect(formatCount(999)).toBe('999')
    expect(formatCount(1000)).toBe('1,000')
    expect(formatCount(12_345)).toBe('12,345')
    // The boundary itself: one below a million must NOT be abbreviated.
    expect(formatCount(999_999)).toBe('999,999')
  })

  it('abbreviates at exactly one million', () => {
    expect(formatCount(1_000_000)).toBe('1.00M')
    expect(formatCount(1_234_567)).toBe('1.23M')
  })

  it('drops decimals as the magnitude grows, keeping three significant digits', () => {
    expect(formatCount(1_230_000)).toBe('1.23M')
    expect(formatCount(12_300_000)).toBe('12.3M')
    expect(formatCount(123_000_000)).toBe('123M')
  })

  it('abbreviates at exactly one billion', () => {
    expect(formatCount(1_000_000_000)).toBe('1.00B')
    expect(formatCount(1_230_000_000)).toBe('1.23B')
    expect(formatCount(25_000_000_000)).toBe('25.0B')
  })

  it('promotes to B rather than rendering a four-digit mantissa', () => {
    // 999,999,999 rounds to 1000M at zero decimals; showing "1.00B" keeps the width
    // stable and matches what compact number formatting does everywhere else.
    expect(formatCount(999_999_999)).toBe('1.00B')
    expect(formatCount(999_999_998)).toBe('1.00B')
  })

  it('handles negatives symmetrically', () => {
    expect(formatCount(-1)).toBe('-1')
    expect(formatCount(-999_999)).toBe('-999,999')
    expect(formatCount(-1_000_000)).toBe('-1.00M')
    expect(formatCount(-2_500_000_000)).toBe('-2.50B')
  })

  it('renders a dash for missing or non-numeric values', () => {
    expect(formatCount(null)).toBe('-')
    expect(formatCount(undefined)).toBe('-')
    expect(formatCount(Number.NaN)).toBe('-')
    expect(formatCount(Number.POSITIVE_INFINITY)).toBe('-')
  })
})

describe('formatCountExact', () => {
  it('always returns the full grouped number, so the title recovers every digit', () => {
    expect(formatCountExact(2_500_000_000)).toBe('2,500,000,000')
    expect(formatCountExact(1_234_567)).toBe('1,234,567')
    expect(formatCountExact(999_999)).toBe('999,999')
    expect(formatCountExact(0)).toBe('0')
    expect(formatCountExact(-1_000_000)).toBe('-1,000,000')
  })

  it('renders a dash for missing values', () => {
    expect(formatCountExact(null)).toBe('-')
    expect(formatCountExact(undefined)).toBe('-')
    expect(formatCountExact(Number.NaN)).toBe('-')
  })

  it('never loses information that formatCount abbreviated away', () => {
    const value = 1_234_567_890
    expect(formatCount(value)).toBe('1.23B')
    expect(formatCountExact(value)).toBe('1,234,567,890')
  })
})

describe('metricValueOf / metricTitleOf', () => {
  const entry = { requests: 1_234_567, tokens: 2_500_000_000, cost_usd: 4321.5 }

  it('abbreviates the two count metrics and leaves cost alone', () => {
    expect(metricValueOf(entry, 'requests')).toBe('1.23M')
    expect(metricValueOf(entry, 'tokens')).toBe('2.50B')
    // Cost must keep its exact $x.xx form — abbreviation is for counts only.
    expect(metricValueOf(entry, 'cost')).toBe('$4321.50')
  })

  it('offers an exact-value tooltip for counts only', () => {
    expect(metricTitleOf(entry, 'requests')).toBe('1,234,567')
    expect(metricTitleOf(entry, 'tokens')).toBe('2,500,000,000')
    expect(metricTitleOf(entry, 'cost')).toBeUndefined()
    expect(metricTitleOf(null, 'tokens')).toBeUndefined()
  })
})
