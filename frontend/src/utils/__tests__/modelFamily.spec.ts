import { describe, expect, it } from 'vitest'
import { aggregateModelFamilies, classifyModelFamily } from '../modelFamily'

describe('classifyModelFamily', () => {
  it('classifies upstream model names into expected families', () => {
    expect(classifyModelFamily('deepseek-flash')).toBe('deepseek')
    expect(classifyModelFamily('deepseek-v4-pro')).toBe('deepseek')
    expect(classifyModelFamily('deepseek/deepseek-reasoner')).toBe('deepseek')
    expect(classifyModelFamily('gpt-5.5')).toBe('openai')
    expect(classifyModelFamily('gpt-6.1-sol')).toBe('openai')
    expect(classifyModelFamily('codex-mini-latest')).toBe('openai')
    expect(classifyModelFamily('o3-mini')).toBe('openai')
    expect(classifyModelFamily('claude-sonnet-4-6')).toBe('claude')
    expect(classifyModelFamily('gemini-2.5-pro')).toBe('gemini')
    expect(classifyModelFamily('grok-4')).toBe('grok')
    expect(classifyModelFamily('qwen3-coder-plus')).toBe('qwen')
    expect(classifyModelFamily('glm-4.5')).toBe('glm')
    expect(classifyModelFamily('kimi-k2')).toBe('kimi')
    expect(classifyModelFamily('custom-local-model')).toBe('other')
    expect(classifyModelFamily('')).toBe('other')
  })
})

describe('aggregateModelFamilies', () => {
  it('groups models by family, sorts by cost desc, and computes share percentages', () => {
    const result = aggregateModelFamilies([
      { model: 'deepseek-flash', requests: 100, tokens: 50000, cost: 1.5 },
      { model: 'deepseek-v4-pro', requests: 20, tokens: 30000, cost: 0.5 },
      { model: 'gpt-6.1-sol', requests: 50, tokens: 120000, cost: 6.0 },
      { model: 'claude-sonnet-4-6', requests: 10, tokens: 40000, cost: 2.0 },
    ])

    expect(result.map((r) => r.family)).toEqual(['openai', 'deepseek', 'claude'])
    expect(result[0]).toMatchObject({
      family: 'openai',
      requests: 50,
      tokens: 120000,
      cost: 6.0,
      sharePct: 60,
      modelsCount: 1,
    })
    expect(result[1]).toMatchObject({
      family: 'deepseek',
      requests: 120,
      tokens: 80000,
      cost: 2.0,
      sharePct: 20,
      modelsCount: 2,
    })
  })

  it('falls back to tokens share when all costs are zero', () => {
    const result = aggregateModelFamilies([
      { model: 'deepseek-flash', requests: 10, tokens: 750, cost: 0 },
      { model: 'gpt-5.5', requests: 5, tokens: 250, cost: 0 },
    ])

    expect(result[0].family).toBe('deepseek')
    expect(result[0].sharePct).toBe(75)
    expect(result[1].family).toBe('openai')
    expect(result[1].sharePct).toBe(25)
  })
})
