import { describe, expect, it } from 'vitest'
import { formatReasoningEffort } from '../format'

describe('formatReasoningEffort', () => {
  it.each(['low', 'medium', 'high', 'xhigh', 'max', 'none', 'minimal', 'adaptive', 'vendor_custom', 'X-High'])(
    'preserves the recorded value %s without merging levels',
    (effort) => expect(formatReasoningEffort(effort)).toBe(effort),
  )

  it.each([null, undefined, '', '   '])('uses a placeholder only for absent effort %s', (effort) => {
    expect(formatReasoningEffort(effort)).toBe('-')
  })

  it('trims surrounding whitespace', () => {
    expect(formatReasoningEffort(' minimal ')).toBe('minimal')
  })
})
