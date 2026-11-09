import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'

import zh from '@/i18n/locales/zh'
import RoutingDiagnosisPanel from '../RoutingDiagnosisPanel.vue'
import RoutingInfoPanel from '../RoutingInfoPanel.vue'
import type { AdminUsageLog } from '@/types'
import {
  attemptsDiagnosisFixture,
  diagnosisFixture,
  loadBalanceInfoFixture,
  switchedInfoFixture
} from './routingFixtures'

const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh } })
const global = { plugins: [i18n], stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } }

describe('RoutingDiagnosisPanel', () => {
  it('renders the header, funnel and check matrix from a diagnosis', () => {
    const wrapper = mount(RoutingDiagnosisPanel, {
      props: { diagnosis: diagnosisFixture, statusCode: 503, requestId: 'req-1' },
      global
    })

    expect(wrapper.get('[data-test="routing-diagnosis-header"]').text()).toBe(
      '分组 Composite（负载均衡）的 4 个账号按顺序检查，任意一关不过就排除'
    )
    const funnel = wrapper.get('[data-test="routing-funnel"]').text()
    expect(funnel).toContain('分组里 4 个账号')
    expect(funnel).toContain('平台对得上 3')
    expect(funnel).toContain('状态正常 2')
    expect(funnel).toContain('支持该模型 0')
    expect(funnel).toContain('没有可用账号 → 503')

    const rows = wrapper.findAll('[data-test="routing-matrix-row"]')
    expect(rows).toHaveLength(4)
    // g1.jacobs：平台/状态/额度过，模型不过，空位没检查
    const first = rows[0]
    expect(first.get('[data-gate="platform"]').text()).toBe('✓')
    expect(first.get('[data-gate="model"]').text()).toBe('✕')
    expect(first.get('[data-gate="capacity"]').text()).toBe('—')
    expect(first.text()).toContain('模型列表里没有 gpt-6.1-sol')
    expect(first.text()).toContain('需要改配置')
    // biz-jacobs：状态不过，后面的关都没检查，带恢复时间
    expect(rows[2].get('[data-gate="schedulable"]').attributes('data-state')).toBe('fail')
    expect(rows[2].get('[data-gate="quota"]').attributes('data-state')).toBe('skip')
    expect(rows[2].text()).toContain('自动恢复')

    expect(wrapper.find('[data-test="routing-attempts"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="routing-fix"]').text()).toContain('没有一个账号开放 gpt-6.1-sol')
    expect(wrapper.get('[data-test="routing-fix"] a').attributes('href')).toBe('/admin/accounts')
    expect(wrapper.get('[data-test="routing-raw"]').text()).toContain('请求 ID req-1')
    expect(wrapper.get('[data-test="routing-raw"]').text()).toContain('filtered: model_not_supported=2')
  })

  it('lists attempts with status, duration and next step', () => {
    const wrapper = mount(RoutingDiagnosisPanel, {
      props: { diagnosis: attemptsDiagnosisFixture, statusCode: 503 },
      global
    })
    const rows = wrapper.findAll('[data-test="routing-attempt-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('第 1 次')
    expect(rows[0].text()).toContain('biz-jianghongbo')
    expect(rows[0].text()).toContain('上游 502')
    expect(rows[0].text()).toContain('3.1s')
    expect(rows[0].text()).toContain('换下一个')
    expect(rows[1].text()).toContain('放弃')
    expect(wrapper.get('[data-test="routing-funnel"]').text()).toContain('都试过仍失败 → 503')
  })

  it('explains an empty pool and points to groups', () => {
    const wrapper = mount(RoutingDiagnosisPanel, {
      props: { diagnosis: { group_name: 'Composite', pool_size: 0, candidates: [], reason: 'empty_pool' }, statusCode: 503 },
      global
    })
    expect(wrapper.get('[data-test="routing-diagnosis-header"]').text()).toBe('分组 Composite 当时一个可用账号都没有，直接返回 503')
    expect(wrapper.find('[data-test="routing-matrix"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="routing-fix"] a').attributes('href')).toBe('/admin/groups')
  })

  it('falls back to "无选号记录" and the raw message when diagnosis is null', () => {
    const wrapper = mount(RoutingDiagnosisPanel, {
      props: { diagnosis: null, statusCode: 502, requestId: 'req-x', message: 'upstream boom' },
      global
    })
    expect(wrapper.get('[data-test="routing-diagnosis-empty"]').text()).toContain('无选号记录')
    expect(wrapper.text()).toContain('upstream boom')
    expect(wrapper.find('[data-test="routing-matrix"]').exists()).toBe(false)
  })
})

describe('RoutingInfoPanel', () => {
  const row = {
    id: 1,
    api_key_id: 5,
    api_key: { id: 5, name: 'jiangjiwei' },
    group: { id: 3, name: 'Composite' },
    account: { id: 1, name: 'g1.jacobs' },
    model: 'gpt-6.1-sol',
    first_token_ms: 1810,
    duration_ms: 2650
  } as unknown as AdminUsageLog

  it('renders the chain with candidates sorted by load and the chosen one highlighted', () => {
    const wrapper = mount(RoutingInfoPanel, { props: { info: loadBalanceInfoFixture, row }, global })
    const chain = wrapper.get('[data-test="routing-chain"]').text()
    expect(chain).toContain('jiangjiwei')
    expect(chain).toContain('Composite')
    expect(chain).toContain('负载均衡')
    const cands = wrapper.findAll('[data-test="routing-candidate"]')
    expect(cands.map((c) => c.attributes('data-chosen'))).toEqual(['true', 'false', 'false'])
    expect(cands[0].text()).toContain('g1.jacobs')
    expect(cands[0].text()).toContain('负载 3/10 · 选中')
    expect(cands[1].text()).toContain('比它忙')
    expect(cands[2].text()).toContain('跳过')
    expect(wrapper.get('[data-test="routing-why"]').text()).toContain('g1.jacobs 当时最闲（3/10）')
    expect(wrapper.find('[data-test="routing-info-attempts"]').exists()).toBe(false)
  })

  it('shows the attempt trail when the request switched accounts', () => {
    const wrapper = mount(RoutingInfoPanel, { props: { info: switchedInfoFixture, row }, global })
    const attempts = wrapper.findAll('[data-test="routing-info-attempt"]')
    expect(attempts).toHaveLength(2)
    expect(attempts[0].text()).toBe('第 1 次 · g1.jacobs · 上游 502（8.2s 后失败）')
    expect(attempts[1].text()).toBe('第 2 次 · biz-jianghongbo · 成功')
    expect(wrapper.get('[data-test="routing-info-attempts"]').text()).toContain('耽误了 8.2s')
  })

  it('shows "无选号记录" when routing info is missing', () => {
    const wrapper = mount(RoutingInfoPanel, { props: { info: null, row }, global })
    expect(wrapper.get('[data-test="routing-info-empty"]').text()).toContain('无选号记录')
  })
})
