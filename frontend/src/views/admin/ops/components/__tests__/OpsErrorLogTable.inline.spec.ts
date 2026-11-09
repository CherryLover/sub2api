import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'

import zh from '@/i18n/locales/zh'
import OpsErrorLogTable from '../OpsErrorLogTable.vue'
import type { OpsErrorLog } from '@/api/admin/ops'
import { diagnosisFixture } from '@/components/admin/usage/__tests__/routingFixtures'

const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })

function row(id: number, extra: Partial<OpsErrorLog> = {}): OpsErrorLog {
  return {
    id,
    created_at: '2026-11-09T01:56:41Z',
    phase: 'routing',
    type: '',
    error_owner: 'platform',
    error_source: 'gateway',
    severity: 'error',
    status_code: 503,
    platform: 'openai',
    model: 'gpt-6.1-sol',
    resolved: false,
    client_request_id: '',
    request_id: `req-${id}`,
    message: 'no available accounts',
    user_email: '',
    account_name: '',
    group_name: 'Composite',
    ...extra,
  } as OpsErrorLog
}

function mountTable(rows: OpsErrorLog[], inlineDiagnosis = true) {
  return mount(OpsErrorLogTable, {
    props: { rows, total: rows.length, loading: false, page: 1, pageSize: 20, inlineDiagnosis },
    global: {
      plugins: [i18n],
      stubs: {
        Pagination: { template: '<div />' },
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

describe('OpsErrorLogTable inline routing diagnosis', () => {
  it('keeps rows collapsed by default and toggles the inline panel with 展开/收起', async () => {
    const wrapper = mountTable([row(1, { routing_diagnosis: diagnosisFixture }), row(2, { routing_diagnosis: null })])

    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(0)
    const toggles = wrapper.findAll('[data-test="routing-toggle"]')
    expect(toggles).toHaveLength(2)
    expect(toggles[0].text()).toContain('展开')

    await toggles[0].trigger('click')
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="routing-matrix"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-test="routing-toggle"]')[0].text()).toContain('收起')

    await wrapper.findAll('[data-test="routing-toggle"]')[1].trigger('click')
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="routing-diagnosis-empty"]').text()).toContain('无选号记录')

    await wrapper.findAll('[data-test="routing-toggle"]')[0].trigger('click')
    expect(wrapper.findAll('[data-test="expanded-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="routing-matrix"]').exists()).toBe(false)
  })

  it('does not open the detail modal on row click in inline mode', async () => {
    const wrapper = mountTable([row(1, { routing_diagnosis: diagnosisFixture })])
    await wrapper.get('tbody tr').trigger('click')
    expect(wrapper.emitted('openErrorDetail')).toBeUndefined()
    await wrapper.get('[data-test="routing-toggle"]').trigger('click')
    expect(wrapper.emitted('openErrorDetail')).toBeUndefined()
  })

  it('renders the reason chip and the candidates summary columns', () => {
    const wrapper = mountTable([
      row(1, { routing_diagnosis: diagnosisFixture }),
      row(2, { routing_diagnosis: null, error_reason: 'empty_pool' }),
      row(3, { routing_diagnosis: null }),
    ])
    const headers = wrapper.findAll('th').map((th) => th.text())
    expect(headers).toEqual(expect.arrayContaining(['原因', '候选账号']))
    const chips = wrapper.findAll('[data-test="routing-reason-chip"]').map((c) => c.text())
    expect(chips).toEqual(['没有账号支持该模型', '分组里没有账号'])
    expect(wrapper.findAll('[data-test="routing-candidates"]').map((c) => c.text())).toEqual(['4 个候选 · 0 个可用'])
  })

  it('keeps the old click-to-open behaviour and columns when not inline', async () => {
    const wrapper = mountTable([row(7, { routing_diagnosis: diagnosisFixture })], false)
    expect(wrapper.find('[data-test="routing-toggle"]').exists()).toBe(false)
    expect(wrapper.findAll('th').map((th) => th.text())).not.toContain('原因')
    await wrapper.get('tbody tr').trigger('click')
    expect(wrapper.emitted('openErrorDetail')?.[0]).toEqual([7])
  })
})
