import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import RankingWindowCard from '../RankingWindowCard.vue'
import type { KeyUsageRankingWindow } from '@/api/keyUsage'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key),
  }),
}))

function mountCard(data: KeyUsageRankingWindow | null) {
  return mount(RankingWindowCard, {
    props: { title: 'Today', windowKey: 'today', data, metric: 'cost' },
    global: { stubs: { RankingTable: true } },
  })
}

function selfEntry(rank: number) {
  return { rank, key_name: 'My Key', requests: 0, tokens: 0, cost_usd: 0, is_self: true }
}

describe('RankingWindowCard', () => {
  // A ranking query that failed must not be rendered as "You rank #1 of 1 Keys":
  // one DB hiccup would otherwise crown every single visitor, and the result is
  // visually indistinguishable from real data.
  it('renders an explicit unavailable state when the backend could not rank', () => {
    const wrapper = mountCard({ total_keys: 0, self_rank: 0, top: [], self: selfEntry(0) })

    expect(wrapper.find('[data-testid="ranking-window-unavailable"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="ranking-self-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ranking-window-empty"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('keyUsage.rankings.unavailable')
    expect(wrapper.text()).not.toContain('keyUsage.rankings.selfRank')
  })

  it('treats a missing payload as unavailable too', () => {
    const wrapper = mountCard(null)
    expect(wrapper.find('[data-testid="ranking-window-unavailable"]').exists()).toBe(true)
  })

  // A Key with no usage on an otherwise empty site really is rank 1 of 1 — that is
  // real data and must stay distinguishable from the failure state above.
  it('renders a real rank of 1 of 1 for the zero-usage-but-ranked case', () => {
    const wrapper = mountCard({ total_keys: 1, self_rank: 1, top: [], self: selfEntry(1) })

    expect(wrapper.find('[data-testid="ranking-window-unavailable"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ranking-self-summary"]').text()).toContain('"rank":1')
    expect(wrapper.find('[data-testid="ranking-self-summary"]').text()).toContain('"total":1')
  })

  it('renders the podium when there are top entries', () => {
    const wrapper = mountCard({
      total_keys: 5,
      self_rank: 2,
      top: [
        { rank: 1, key_name: 'Alpha', requests: 1, tokens: 1, cost_usd: 1, is_self: false },
        { rank: 2, key_name: 'My Key', requests: 1, tokens: 1, cost_usd: 1, is_self: true },
      ],
      self: selfEntry(2),
    })

    expect(wrapper.find('[data-testid="ranking-window-unavailable"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="podium-slot"]')).toHaveLength(2)
  })

  // Rankings worked, this Key just is not in this window: distinct from unavailable.
  it('renders the empty state when ranking data exists but this Key is unranked', () => {
    const wrapper = mountCard({ total_keys: 12, self_rank: 0, top: [], self: null })

    expect(wrapper.find('[data-testid="ranking-window-unavailable"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ranking-window-empty"]').exists()).toBe(true)
  })
})
