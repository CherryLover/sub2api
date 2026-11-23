import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Account } from '@/types'
import OpenAICreditsBarChart from '../OpenAICreditsBarChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const makeAccount = (id: number, name: string, extra: Record<string, unknown>): Account =>
  ({
    id,
    name,
    platform: 'openai',
    type: 'oauth',
    extra,
  }) as unknown as Account

describe('OpenAICreditsBarChart', () => {
  it('renders horizontal remaining + spent bars sorted by balance and hides accounts without credits', () => {
    const accounts = [
      makeAccount(1, 'acct-low', {
        codex_credits_has: true,
        codex_credits_balance: 200,
        codex_credits_peak_balance: 500,
        codex_credits_mode: 'last',
      }),
      makeAccount(2, 'acct-high', {
        codex_credits_has: true,
        codex_credits_balance: 800,
        codex_credits_peak_balance: 1000,
        codex_credits_mode: 'first',
      }),
      makeAccount(3, 'acct-none', {}),
    ]

    const wrapper = mount(OpenAICreditsBarChart, { props: { accounts } })

    const rows = wrapper.findAll('[data-testid="credits-chart-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].attributes('data-account-id')).toBe('2')
    expect(rows[1].attributes('data-account-id')).toBe('1')

    expect(rows[0].find('[data-testid="credits-bar-remaining"]').exists()).toBe(true)
    expect(rows[0].find('[data-testid="credits-bar-spent"]').exists()).toBe(true)
    expect(rows[0].text()).toContain('800')
    expect(rows[0].text()).toContain('-200')
    expect(rows[0].text()).toContain('admin.accounts.creditsMode.first')
  })

  it('does not render when no OpenAI OAuth accounts have credits data', () => {
    const wrapper = mount(OpenAICreditsBarChart, {
      props: { accounts: [makeAccount(1, 'plain', {})] },
    })
    expect(wrapper.find('[data-testid="openai-credits-bar-chart"]').exists()).toBe(false)
  })
})
