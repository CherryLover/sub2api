import { afterEach, describe, expect, it } from 'vitest'

import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

describe('usePersistedPageSize', () => {
  afterEach(() => {
    localStorage.clear()
  })

  it('falls back to the built-in default when nothing is persisted', () => {
    expect(getPersistedPageSize()).toBe(20)
  })

  it('normalizes a persisted value against the built-in options', () => {
    localStorage.setItem('table-page-size', '35')

    expect(getPersistedPageSize()).toBe(50)
  })

  it('clamps a persisted value larger than the largest option', () => {
    localStorage.setItem('table-page-size', '1000')

    expect(getPersistedPageSize()).toBe(100)
  })
})
