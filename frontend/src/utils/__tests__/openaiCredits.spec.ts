import { describe, expect, it } from 'vitest'
import {
  formatCreditsAmount,
  getOpenAICreditsInfo,
  normalizeCodexCreditsMode,
} from '../openaiCredits'

describe('normalizeCodexCreditsMode', () => {
  it('normalizes known modes and defaults to on', () => {
    expect(normalizeCodexCreditsMode('first')).toBe('first')
    expect(normalizeCodexCreditsMode(' LAST ')).toBe('last')
    expect(normalizeCodexCreditsMode('off')).toBe('off')
    expect(normalizeCodexCreditsMode('on')).toBe('on')
    expect(normalizeCodexCreditsMode('invalid')).toBe('on')
    expect(normalizeCodexCreditsMode(undefined)).toBe('on')
  })
})

describe('getOpenAICreditsInfo', () => {
  it('computes balance, peak, spent, and percentages', () => {
    const info = getOpenAICreditsInfo({
      platform: 'openai',
      type: 'oauth',
      extra: {
        codex_credits_has: true,
        codex_credits_unlimited: false,
        codex_credits_balance: 750,
        codex_credits_peak_balance: 1000,
        codex_credits_mode: 'last',
      },
    })

    expect(info).toMatchObject({
      hasData: true,
      hasCredits: true,
      unlimited: false,
      balance: 750,
      peakBalance: 1000,
      spent: 250,
      remainingPct: 75,
      spentPct: 25,
      mode: 'last',
      usable: true,
    })
  })

  it('handles unlimited credits', () => {
    const info = getOpenAICreditsInfo({
      platform: 'openai',
      type: 'oauth',
      extra: {
        codex_credits_has: true,
        codex_credits_unlimited: true,
        codex_credits_balance: 0,
      },
    })

    expect(info.unlimited).toBe(true)
    expect(info.usable).toBe(true)
    expect(info.remainingPct).toBe(100)
    expect(info.spent).toBe(0)
  })
})

describe('formatCreditsAmount', () => {
  it('formats small and large credit numbers cleanly', () => {
    expect(formatCreditsAmount(0)).toBe('0')
    expect(formatCreditsAmount(42.5)).toBe('42.5')
    expect(formatCreditsAmount(1234.56)).toBe('1234.6')
    expect(formatCreditsAmount(28263.23)).toBe('28.26K')
  })
})
