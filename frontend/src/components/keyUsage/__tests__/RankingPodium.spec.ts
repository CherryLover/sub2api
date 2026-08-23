import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import RankingPodium from '../RankingPodium.vue'
import type { KeyUsageRankingEntry } from '@/api/keyUsage'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

function entry(overrides: Partial<KeyUsageRankingEntry> = {}): KeyUsageRankingEntry {
  return {
    rank: 1,
    key_name: 'Key',
    requests: 1,
    tokens: 2,
    cost_usd: 3,
    is_self: false,
    ...overrides,
  }
}

function slotsOf(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('[data-testid="podium-slot"]')
}

describe('RankingPodium', () => {
  it('orders three winners as silver, gold, bronze', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [
          entry({ rank: 1, key_name: 'Gold Key' }),
          entry({ rank: 2, key_name: 'Silver Key' }),
          entry({ rank: 3, key_name: 'Bronze Key' }),
        ],
      },
    })

    const slots = slotsOf(wrapper)
    expect(slots.map(s => s.attributes('data-medal'))).toEqual(['silver', 'gold', 'bronze'])
    expect(slots.map(s => s.attributes('data-rank'))).toEqual(['2', '1', '3'])
  })

  it('gives gold the tallest pedestal and bronze the shortest', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [entry({ rank: 1 }), entry({ rank: 2 }), entry({ rank: 3 })],
      },
    })

    const slots = slotsOf(wrapper)
    const heightOf = (index: number) =>
      (slots[index].element.querySelector('div.rounded-b-xl') as HTMLElement).className
    expect(heightOf(1)).toContain('sm:h-24') // gold
    expect(heightOf(0)).toContain('sm:h-16') // silver
    expect(heightOf(2)).toContain('sm:h-10') // bronze
  })

  it('renders a partial podium when fewer than three keys have usage', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [entry({ rank: 1, key_name: 'Only' }), entry({ rank: 2, key_name: 'Second' })],
      },
    })

    const slots = slotsOf(wrapper)
    expect(slots.map(s => s.attributes('data-medal'))).toEqual(['silver', 'gold'])
  })

  it('falls back to array position when the payload omits rank', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [
          { key_name: 'A', requests: 1, tokens: 1, cost_usd: 1, is_self: false },
          { key_name: 'B', requests: 1, tokens: 1, cost_usd: 1, is_self: false },
          { key_name: 'C', requests: 1, tokens: 1, cost_usd: 1, is_self: false },
        ] as unknown as KeyUsageRankingEntry[],
      },
    })

    expect(slotsOf(wrapper).map(s => s.attributes('data-medal'))).toEqual(['silver', 'gold', 'bronze'])
  })

  it('shows the value of the active metric', () => {
    const entries = [entry({ rank: 1, requests: 42, tokens: 123456, cost_usd: 7.5 })]

    const cost = mount(RankingPodium, { props: { metric: 'cost', entries } })
    expect(cost.text()).toContain('$7.50')

    const tokens = mount(RankingPodium, { props: { metric: 'tokens', entries } })
    expect(tokens.text()).toContain('123,456')

    const requests = mount(RankingPodium, { props: { metric: 'requests', entries } })
    expect(requests.text()).toContain('42')
  })

  // The backend uses standard competition ranking (1224): ties share a rank and the next
  // rank is skipped. De-duplicating by rank would blank out a whole medal slot and silently
  // drop a Key that is actually on the podium.
  it('keeps both winners of a tie in the gold slot', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [
          entry({ rank: 1, key_name: 'Gold A' }),
          entry({ rank: 1, key_name: 'Gold B' }),
          entry({ rank: 3, key_name: 'Bronze Key' }),
        ],
      },
    })

    const slots = slotsOf(wrapper)
    expect(slots.map(s => s.attributes('data-medal'))).toEqual(['gold', 'bronze'])
    expect(wrapper.findAll('[data-testid="podium-entry"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('Gold A')
    expect(wrapper.text()).toContain('Gold B')
    expect(wrapper.text()).toContain('Bronze Key')
    expect(slots[0].attributes('data-tied')).toBe('true')
  })

  it('keeps both runners-up when second place is tied and third is skipped', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [
          entry({ rank: 1, key_name: 'Gold Key' }),
          entry({ rank: 2, key_name: 'Silver A' }),
          entry({ rank: 2, key_name: 'Silver B' }),
        ],
      },
    })

    // Silver stays on the left, gold in the middle; the bronze slot is simply absent
    // because rank 3 does not exist under 1224 ranking after a two-way tie for 2nd.
    const slots = slotsOf(wrapper)
    expect(slots.map(s => s.attributes('data-medal'))).toEqual(['silver', 'gold'])
    expect(wrapper.findAll('[data-testid="podium-entry"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('Silver A')
    expect(wrapper.text()).toContain('Silver B')
  })

  it('keeps a three-way tie for first together and drops nobody', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [
          entry({ rank: 1, key_name: 'A' }),
          entry({ rank: 1, key_name: 'B' }),
          entry({ rank: 1, key_name: 'C' }),
        ],
      },
    })

    expect(slotsOf(wrapper).map(s => s.attributes('data-medal'))).toEqual(['gold'])
    expect(wrapper.findAll('[data-testid="podium-entry"]')).toHaveLength(3)
    for (const name of ['A', 'B', 'C']) {
      expect(wrapper.text()).toContain(name)
    }
  })

  it('marks the self entry inside a tied slot', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [
          entry({ rank: 1, key_name: 'Someone else' }),
          entry({ rank: 1, key_name: 'Mine', is_self: true }),
        ],
      },
    })

    expect(wrapper.findAll('[data-testid="podium-self-badge"]')).toHaveLength(1)
  })

  it('ignores entries ranked outside the podium', () => {
    const wrapper = mount(RankingPodium, {
      props: {
        metric: 'cost',
        entries: [entry({ rank: 1, key_name: 'Gold' }), entry({ rank: 4, key_name: 'Fourth' })],
      },
    })

    expect(wrapper.findAll('[data-testid="podium-entry"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('Fourth')
  })

  it('renders an empty state when there is nothing to show', () => {
    const wrapper = mount(RankingPodium, { props: { metric: 'cost', entries: [] } })

    expect(slotsOf(wrapper)).toHaveLength(0)
    expect(wrapper.find('[data-testid="podium-empty"]').exists()).toBe(true)
  })
})
