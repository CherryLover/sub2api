import { describe, expect, it } from 'vitest'

import {
  DEFAULT_TABLE_PAGE_SIZE,
  DEFAULT_TABLE_PAGE_SIZE_OPTIONS,
  normalizeTablePageSize
} from '@/utils/tablePreferences'

describe('tablePreferences', () => {
  it('keeps built-in selectable defaults at 10, 20, 50, 100', () => {
    expect(DEFAULT_TABLE_PAGE_SIZE).toBe(20)
    expect(DEFAULT_TABLE_PAGE_SIZE_OPTIONS).toEqual([10, 20, 50, 100])
  })

  it('normalizes page size against the built-in options by rounding up', () => {
    expect(normalizeTablePageSize(10)).toBe(10)
    expect(normalizeTablePageSize(35)).toBe(50)
    expect(normalizeTablePageSize(100)).toBe(100)
  })

  it('clamps anything above the largest option down to it', () => {
    expect(normalizeTablePageSize(1000)).toBe(100)
    expect(normalizeTablePageSize(1500)).toBe(100)
  })

  it('falls back to the built-in default for invalid input', () => {
    expect(normalizeTablePageSize(undefined)).toBe(DEFAULT_TABLE_PAGE_SIZE)
    expect(normalizeTablePageSize('abc')).toBe(DEFAULT_TABLE_PAGE_SIZE)
    expect(normalizeTablePageSize(2)).toBe(DEFAULT_TABLE_PAGE_SIZE)
  })
})
