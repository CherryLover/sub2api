import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import KeyModelHeatmap from '../KeyModelHeatmap.vue'
import { createDashboardKey, metric } from './fixtures'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const message = key === 'keyUsageDashboard.keyModel.showAll'
          ? 'Show all {total} keys'
          : key
        return params
          ? message.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name]))
          : message
      }
    })
  }
})

describe('KeyModelHeatmap', () => {
  it('shows the top 10 keys by default and expands to all keys', async () => {
    const keys = Array.from({ length: 12 }, (_, index) => createDashboardKey(index + 1))
    const wrapper = mount(KeyModelHeatmap, {
      props: {
        keys,
        models: {
          names: ['gpt-5'],
          keys: keys.map((key) => ({
            api_key_id: key.api_key_id,
            cells: [metric(key.totals.cost, key.totals.requests, key.totals.tokens)]
          }))
        },
        metric: 'requests'
      }
    })

    expect(wrapper.findAll('tbody tr')).toHaveLength(10)
    const expand = wrapper.get('[data-testid="key-model-expand"]')
    expect(expand.text()).toContain('Show all 12 keys')

    await expand.trigger('click')

    expect(wrapper.findAll('tbody tr')).toHaveLength(12)
  })
})
